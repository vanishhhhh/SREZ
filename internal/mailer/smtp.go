package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"html/template"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"time"

	"srez/internal/config"
	"srez/internal/security"
)

type SMTP struct{ cfg config.Mail }

func NewSMTP(c config.Mail) *SMTP { return &SMTP{cfg: c} }

const textBody = "Спасибо за регистрацию в SREZ!\n\nПодтвердите адрес электронной почты, чтобы активировать аккаунт.\n\nДля подтверждения перейдите по ссылке ниже.\n\nСсылка действительна 30 минут.\n\nЕсли вы не регистрировались в SREZ, просто проигнорируйте это письмо.\n\n"

var htmlBody = template.Must(template.New("verification").Parse(`<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="referrer" content="no-referrer"></head><body style="margin:0;background:#f5f4ef;color:#171715;font-family:Arial,sans-serif"><main style="max-width:560px;margin:40px auto;padding:40px;background:white;border:1px solid #ddd"><div style="font-size:42px;font-weight:900;letter-spacing:-3px">SREZ®</div><p style="font-size:11px;letter-spacing:2px;margin-top:32px">СВОЙ КРУГ. СВОЙ ВЫБОР.</p><h1 style="font-size:30px">Подтвердите почту.</h1><p>Спасибо за регистрацию в SREZ!</p><p>Подтвердите адрес электронной почты, чтобы активировать аккаунт.</p><p>Для подтверждения перейдите по ссылке ниже.</p><p style="margin:32px 0"><a href="{{.}}" style="display:inline-block;background:#171715;color:#fff;padding:18px 26px;text-decoration:none;font-weight:bold" rel="noreferrer">Подтвердить почту</a></p><p>Ссылка действительна 30 минут.</p><p style="color:#666">Если вы не регистрировались в SREZ, просто проигнорируйте это письмо.</p></main></body></html>`))

func build(from, to, link string) ([]byte, error) {
	return buildMessage(from, to, link, false)
}
func buildMessage(from, to, link string, recovery bool) ([]byte, error) {
	text, subject, html := textBody, "Подтвердите email — SREZ", htmlBody
	if recovery {
		text, subject, html = recoveryText, "Сброс пароля — SREZ", recoveryHTML
	}
	var out, body bytes.Buffer
	m := multipart.NewWriter(&body)
	for _, kind := range []string{"text/plain", "text/html"} {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", kind+`; charset="UTF-8"`)
		h.Set("Content-Transfer-Encoding", "quoted-printable")
		part, e := m.CreatePart(h)
		if e != nil {
			return nil, e
		}
		w := quotedprintable.NewWriter(part)
		if kind == "text/plain" {
			_, e = w.Write([]byte(text + link + "\n"))
		} else {
			e = html.Execute(w, link)
		}
		if e != nil {
			return nil, e
		}
		if e = w.Close(); e != nil {
			return nil, e
		}
	}
	if e := m.Close(); e != nil {
		return nil, e
	}
	headers := []struct{ k, v string }{{"From", from}, {"To", to}, {"Subject", mime.QEncoding.Encode("UTF-8", subject)}, {"Date", time.Now().UTC().Format(time.RFC1123Z)}, {"Message-ID", "<" + security.Random() + "@srez.local>"}, {"MIME-Version", "1.0"}, {"Content-Type", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": m.Boundary()})}}
	for _, h := range headers {
		out.WriteString(h.k + ": " + h.v + "\r\n")
	}
	out.WriteString("\r\n")
	out.Write(body.Bytes())
	return out.Bytes(), nil
}
func (s *SMTP) SendVerification(ctx context.Context, to, link string) error {
	return s.send(ctx, to, link, false)
}
func (s *SMTP) SendPasswordReset(ctx context.Context, to, link string) error {
	return s.send(ctx, to, link, true)
}
func (s *SMTP) send(ctx context.Context, to, link string, recovery bool) error {
	fail := errors.New("smtp delivery failed")
	recipient, e := mail.ParseAddress(to)
	if e != nil || recipient.Address != to {
		return fail
	}
	from, e := mail.ParseAddress(s.cfg.From)
	if e != nil {
		return fail
	}
	message, e := buildMessage(from.String(), recipient.String(), link, recovery)
	if e != nil {
		return fail
	}
	address := net.JoinHostPort(s.cfg.Host, s.cfg.Port)
	conn, e := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", address)
	if e != nil {
		return fail
	}
	baseConn := conn
	defer baseConn.Close()
	deadline := time.Now().Add(5 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	if e = conn.SetDeadline(deadline); e != nil {
		return fail
	}
	stop := context.AfterFunc(ctx, func() { baseConn.Close() })
	defer stop()
	tlsCfg := &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}
	if s.cfg.TLSMode == "tls" {
		secure := tls.Client(conn, tlsCfg)
		if e = secure.HandshakeContext(ctx); e != nil {
			return fail
		}
		conn = secure
	}
	client, e := smtp.NewClient(conn, s.cfg.Host)
	if e != nil {
		return fail
	}
	defer client.Close()
	if s.cfg.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fail
		}
		if e = client.StartTLS(tlsCfg); e != nil {
			return fail
		}
	}
	if s.cfg.Username != "" {
		if e = client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); e != nil {
			return fail
		}
	}
	if e = client.Mail(from.Address); e != nil {
		return fail
	}
	if e = client.Rcpt(to); e != nil {
		return fail
	}
	w, e := client.Data()
	if e != nil {
		return fail
	}
	if _, e = w.Write(message); e != nil {
		return fail
	}
	if e = w.Close(); e != nil {
		return fail
	}
	// DATA's successful final reply means the SMTP server accepted the message.
	_ = client.Quit()
	return nil
}
