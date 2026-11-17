package mailer

import (
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

func TestRecoveryMessageAlternativesAndPurpose(t *testing.T) {
	data, e := buildMessage("SREZ <no-reply@srez.local>", "recipient@example.test", "http://localhost:5175/reset-password#token=test", true)
	if e != nil {
		t.Fatal(e)
	}
	msg, e := mail.ReadMessage(strings.NewReader(string(data)))
	if e != nil {
		t.Fatal(e)
	}
	subject, e := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if e != nil || subject != "Сброс пароля — SREZ" {
		t.Fatal("recovery subject")
	}
	kind, params, e := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if e != nil || kind != "multipart/alternative" {
		t.Fatal("recovery MIME")
	}
	reader := multipart.NewReader(msg.Body, params["boundary"])
	count := 0
	for {
		part, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(part)
		if e != nil {
			t.Fatal(e)
		}
		text := string(b)
		if !strings.Contains(text, "30 минут") || !strings.Contains(text, "reset-password#token=test") || strings.Contains(text, "<script") || strings.Contains(text, "<img") || strings.Contains(text, "verify-email") {
			t.Fatal("recovery content/purpose")
		}
		count++
	}
	if count != 2 {
		t.Fatal("missing recovery text/HTML")
	}
}
