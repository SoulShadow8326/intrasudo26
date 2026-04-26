package handlers

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
)

type SMTPMailer struct {
	Host string
	Port string
	User string
	Pass string
}

func NewSMTPMailerFromEnv() *SMTPMailer {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	user := strings.TrimSpace(os.Getenv("SMTP_USER"))
	pass := strings.TrimSpace(os.Getenv("SMTP_PASS"))
	if port == "" {
		port = "25"
	}
	return &SMTPMailer{Host: host, Port: port, User: user, Pass: pass}
}

func (s *SMTPMailer) Send(to, subject, html string) error {
	if s == nil || s.Host == "" {
		fmt.Printf("smtp disabled; mail to=%s subject=%s bytes=%d\n", to, subject, len(html))
		return nil
	}

	addr := net.JoinHostPort(s.Host, s.Port)

	from := s.User
	if from == "" {
		from = "no-reply@" + s.Host
	}
	msg := "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"\r\n" + html

	auth := smtp.PlainAuth("", s.User, s.Pass, s.Host)

	if s.Port == "465" {
		tlsconfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         s.Host,
		}
		conn, err := tls.Dial("tcp", addr, tlsconfig)
		if err != nil {
			return err
		}
		c, err := smtp.NewClient(conn, s.Host)
		if err != nil {
			return err
		}
		defer c.Quit()
		if s.User != "" {
			if err = c.Auth(auth); err != nil {
				return err
			}
		}
		if err = c.Mail(from); err != nil {
			return err
		}
		if err = c.Rcpt(to); err != nil {
			return err
		}
		w, err := c.Data()
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(msg))
		if err != nil {
			return err
		}
		err = w.Close()
		if err != nil {
			return err
		}
		return c.Quit()
	}

	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}
