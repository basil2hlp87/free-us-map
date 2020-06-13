package verify

import (
	"fmt"
	"net/smtp"
	"os"
)

func sendMessage(to string, params string) error {
	from := os.Getenv("VERIFY_MAIL_SENDER")
	subject := "freeUSmap.live email verification"
	body := fmt.Sprintf("Visit %s/verify?%s to finish verification", os.Getenv("BASE_URL"), params)
	msg := []byte(fmt.Sprintf("To: %s\r\nSubject: %s\r\n\r\n%s.\r\n", to, subject, body))

	return smtp.SendMail(addr(), auth(), from, []string{to}, msg)
}

func addr() string {
	return os.Getenv("HOST_NAME") + ":" + os.Getenv("SMTP_PORT")
}

func auth() smtp.Auth {
	hostname := os.Getenv("HOST_NAME")
	plainAuthzId := os.Getenv("PLAIN_AUTHZ_ID")
	plainAuthcId := os.Getenv("PLAIN_AUTHC_ID")
	plainPassword := os.Getenv("PLAIN_PASSWORD")

	return smtp.PlainAuth(plainAuthzId, plainAuthcId, plainPassword, hostname)
}
