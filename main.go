package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"

	"./src/geo"
)

var envFile string
var portNum int
var rootDir string

func init() {
	flag.StringVar(&envFile, "env-file", ".env", "File containing environment variables")
	flag.IntVar(&portNum, "port", 8999, "Port to listen to incoming requests to this application on")
	flag.StringVar(&rootDir, "asset-dir", "./static", "Root directory containing static assets for front-end")
}

func main() {
	flag.Parse()

	// TODO: Can we change values in here on the fly? If not, let's move this
	// into a function that gets called and loads the file each time.
	err := godotenv.Load(envFile)
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("postgres", os.Getenv("DATABASE_DSN"))
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/api/v1/downvote", handlePointVote(db, -1))
	http.HandleFunc("/api/v1/upvote", handlePointVote(db, 1))
	http.HandleFunc("/api/v1/delete", handleDeletePoint(db))
	http.HandleFunc("/api/v1/point", handlePostPoint(db))
	http.HandleFunc("/api/v1/points", handleGetPoints(db))
	http.Handle("/", http.FileServer(http.Dir(rootDir)))

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(portNum), http.DefaultServeMux))
}

// LIST POINTS

func handleGetPoints(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		bnd, err := boundsFromRequest(r)
		if err != nil {
			log.Print(err)
			return
		}

		qry := `select
		  coordinates,
		  id,
		  body,
		  created_at,
		  icon,
		  case created_by when $5 then true else false end
		from points
		where created_at > now() - interval '12 hours' 
		  and box(point($1, $2), point($3, $4)) @> coordinates
		  and hidden = false
		limit 500;`
		rows, err := db.Query(qry, bnd.Ne.X(), bnd.Ne.Y(), bnd.Sw.X(), bnd.Sw.Y(), bnd.RequestedBy)
		if err != nil {
			log.Print(err)
			return
		}
		defer rows.Close()

		pts := make([]geo.Element, 0)
		for rows.Next() {
			var pt geo.PsqlPoint
			var id, msg, icon string
			var createdAt time.Time
			var canDelete bool
			err = rows.Scan(&pt, &id, &msg, &createdAt, &icon, &canDelete)
			if err != nil {
				log.Print(err)
				return
			}

			msg = toClickableLink(msg)

			pts = append(pts, geo.NewPointFromCoords(pt.Point, id, msg, createdAt, icon, canDelete))
		}
		// Check for errors from iterating over rows.
		if err := rows.Err(); err != nil {
			log.Print(err)
			return
		}

		b, err := json.Marshal(pts)
		if err != nil {
			log.Print(err)
			return
		}

		w.Write(b)
	}
}

func toClickableLink(msg string) string {
	s := strings.TrimSpace(msg)

	u, err := url.Parse(s)
	if err != nil {
		return msg
	}

	switch u.Host {
	case "twitter.com", "mobile.twitter.com":
		return fmt.Sprintf("<a href=\"%s\" target=\"_blank\">%s</a>", s, msg)
	}

	return msg
}

type coord struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lng"`
}

func (c *coord) X() float64 { return c.Longitude }
func (c *coord) Y() float64 { return c.Latitude }

type bounds struct {
	RequestedBy string `json:"requested_by"`
	Ne          coord  `json:"NE"`
	Sw          coord  `json:"SW"`
}

func boundsFromRequest(r *http.Request) (bnd *bounds, err error) {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot read body from request")
	}

	err = json.Unmarshal(body, &bnd)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot unpack request body into struct")
	}

	return bnd, nil
}

// CREATE NEW POINT

type newPoint struct {
	Coords    coord  `json:"coords"`
	CreatedBy string `json:"created_by"`
	Message   string `json:"message"`
	Icon      string `json:"icon"`
}

func handlePostPoint(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		pt, err := newPointFromRequest(r)
		if err != nil {
			log.Print(err)
			return
		}

		var id string
		qry := "insert into points (coordinates, body, icon, created_at, created_by) values (point($1, $2), $3, $4, now(), $5) returning id"
		err = db.QueryRow(qry, pt.Coords.X(), pt.Coords.Y(), pt.Message, pt.Icon, pt.CreatedBy).Scan(&id)
		if err != nil {
			log.Print(err)
			return
		}

		xy := [2]float64{pt.Coords.X(), pt.Coords.Y()}
		pts := []geo.Element{geo.NewPointFromCoords(xy, id, pt.Message, time.Now(), pt.Icon, true)}

		b, err := json.Marshal(pts)
		if err != nil {
			log.Print(err)
			return
		}

		w.Write(b)
	}
}

func newPointFromRequest(r *http.Request) (pt *newPoint, err error) {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot read body from request")
	}

	err = json.Unmarshal(body, &pt)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot unpack request body into struct")
	}

	return pt, nil
}

// DELETE POINT

type deleteOpts struct {
	Id        int    `json:"point_id"`
	CreatedBy string `json:"created_by"`
}

func handleDeletePoint(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		opts, err := newDeleteOptsFromRequest(r)
		if err != nil {
			log.Print(err)
			return
		}

		qry := "update points set hidden = true where id = $1 and created_by = $2"
		_, err = db.Exec(qry, opts.Id, opts.CreatedBy)
		if err != nil {
			log.Print(err)
			return
		}
	}
}

func newDeleteOptsFromRequest(r *http.Request) (opts *deleteOpts, err error) {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot read body from request")
	}

	err = json.Unmarshal(body, &opts)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot unpack request body into struct")
	}

	return opts, nil
}

// UP/DOWNVOTE POINT

type voteOpts struct {
	PointId int    `json:"point_id"`
	Voter   string `json:"voter"`
}

func handlePointVote(db *sql.DB, val int) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		opts, err := newVoteOptsFromRequest(r)
		if err != nil {
			log.Print(err)
			return
		}

		// Check if this person has already voted.
		qry := "insert into votes (voter_id, point_id, value) values ($1, $2, $3)"
		_, err = db.Exec(qry, opts.Voter, opts.PointId, val)
		if err != nil {
			// Assume we hit a duplicate key constraint and return early.
			return
		}

		go maybeHidePoint(db, opts.PointId)
	}
}

func maybeHidePoint(db *sql.DB, id int) {
	var score int
	qry := "select sum(value) from votes where point_id = $1"
	err := db.QueryRow(qry, id).Scan(&score)
	if err != nil {
		log.Print(err)
		return
	}

	// Set our threshold at -5 for now. Improve how this is stored later.
	if score <= -5 {
		qry := "update points set hidden = true where id = $1"
		_, err = db.Exec(qry, id)
		if err != nil {
			log.Print(err)
			return
		}
	}
}

func newVoteOptsFromRequest(r *http.Request) (opts *voteOpts, err error) {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot read body from request")
	}

	err = json.Unmarshal(body, &opts)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot unpack request body into struct")
	}

	return opts, nil
}
