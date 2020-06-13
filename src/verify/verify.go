package verify

import (
	"crypto/sha512"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type sendVerificationBody struct {
	EmailAddress string `json:"email"`
	CurrentUrl   string `json:"current_url"`
}

func addressFromRequest(r *http.Request) (b *sendVerificationBody, err error) {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot read body from request")
	}

	err = json.Unmarshal(body, &b)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot unpack request body into struct")
	}

	return b, nil
}

func normalizeGmailLocalPart(local string) string {
	// Remove arbitrary periods gmail allows in local part.
	local = strings.Replace(local, ".", "", -1)

	// Remove any tags gmail allows after + characters.
	local = strings.Split(local, "+")[0]

	return local
}

func partsFromEmailAddress(addr string) (string, string, error) {
	addr = strings.TrimSpace(addr)
	parts := strings.Split(addr, "@")

	if len(parts) < 2 {
		return "", "", errors.Errorf("Invalid email address: %s", addr)
	}

	host := parts[len(parts)-1]
	localParts := parts[:len(parts)-1]
	local := strings.Join(localParts, "@")

	// Periods and + characters are rare enough that we're going to normalize
	// all email addresses as if they were gmail for now.
	//
	// if host == "gmail.com" {
	local = normalizeGmailLocalPart(local)
	// }

	return strings.ToLower(local), strings.ToLower(host), nil
}

func HandleSendVerification(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := addressFromRequest(r)
		if err != nil {
			log.Print(err)
			http.Error(w, err.Error(), 400)
			return
		}
		addr := b.EmailAddress

		// Validate email address.
		local, host, err := partsFromEmailAddress(addr)
		if err != nil {
			log.Print(err)
			http.Error(w, err.Error(), 400)
			return
		}

		// Hash the local and host parts. Using static salt is fine here since
		// the intent is only to obscure the address, not be crypto-secure.
		salt := os.Getenv("EMAIL_ADDR_SALT")

		// TODO: This is likely quite inefficient, but it's good enough for now.
		// Improve it later.
		uHasher := sha512.New()
		_, err = uHasher.Write([]byte(local + salt))
		if err != nil {
			log.Print(err)
			http.Error(w, err.Error(), 500)
			return
		}
		userHash := base64.URLEncoding.EncodeToString(uHasher.Sum(nil))

		dHasher := sha512.New()
		_, err = dHasher.Write([]byte(host + salt))
		if err != nil {
			log.Print(err)
			http.Error(w, err.Error(), 500)
			return
		}
		domainHash := base64.URLEncoding.EncodeToString(dHasher.Sum(nil))

		// Check if this email address has been banned.
		var banned bool
		qry := "select banned from users where username = $1 and domain = $2"
		err = db.QueryRow(qry, userHash, domainHash).Scan(&banned)
		if banned {
			http.Error(w, "Banned", 403)
			return
		}

		// Create a new verification code for this person.
		code := uuid.New().String()
		qry = `insert into users (username, domain, code, code_created_at)
			   values ($1, $2, $3, now())
			   on conflict (username, domain) do update
			   set (code, code_created_at) = ($3, now())`
		_, err = db.Exec(qry, userHash, domainHash, code)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Set other params in the verification code so that the browser
		// returns to the same place on the map.
		params := validParamsForRedirectUrl(b.CurrentUrl)
		params.Add("code", code)
		err = sendMessage(addr, params.Encode())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		w.Write([]byte("Verification email sent"))
	}
}

func validParamsForRedirectUrl(s string) url.Values {
	u, err := url.Parse(s)
	if err != nil {
		return url.Values{}
	}

	validUrlParams := map[string]bool{
		"lat": true,
		"lng": true,
		"zm":  true,
	}

	v := u.Query()
	for key, _ := range v {
		_, exists := validUrlParams[key]
		if !exists {
			v.Del(key)
		}
	}

	return v
}

func HandleVerifyCode(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		u := r.URL
		code := u.Query().Get("code")

		// Return early if the verification code is invalid.
		var userId int
		qry := "select id from users where code = $1 and code_created_at > now() - interval '15 minutes'"
		err := db.QueryRow(qry, code).Scan(&userId)
		if userId == 0 {
			http.Error(w, "Invalid verification code", 400)
			return
		}

		// Insert the cookie into the database.
		cookieVal := uuid.New().String()
		qry = `insert into cookies (user_id, cookie, created_at) values ($1, $2, now())`
		_, err = db.Exec(qry, userId, cookieVal)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Redirect the response.
		redirectTo, err := url.Parse(os.Getenv("BASE_URL"))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// Set the cookie on the response.
		cookie := &http.Cookie{
			Name:   "free-us-map",
			Value:  cookieVal,
			Domain: redirectTo.Host,
			// MaxAge: 60 * 60 * 24 * 3, // 3 days
		}
		http.SetCookie(w, cookie)

		q := redirectTo.Query()
		for key, val := range r.URL.Query() {
			// Include all parameters in the redirect except the code param.
			if key != "code" {
				q.Set(key, val[0])
			}
		}
		redirectTo.RawQuery = q.Encode()

		http.Redirect(w, r, redirectTo.String(), 302)
	}
}

func HandleVerificationCheck(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("free-us-map")
		if err == nil && CookieIsGood(db, cookie.Value) {
			w.Write([]byte("{\"verified\": true}"))
			return
		}

		w.Write([]byte("{\"verified\": false}"))
		return
	}
}

func CookieIsGood(db *sql.DB, cookie string) bool {
	var userId int
	var allowed bool

	qry := `select users.id, not users.banned
	from cookies
	join users
	  on users.id = cookies.user_id
	 where cookies.cookie = $1`
	err := db.QueryRow(qry, cookie).Scan(&userId, &allowed)
	if err != nil || userId == 0 {
		return false
	}

	return allowed
}
