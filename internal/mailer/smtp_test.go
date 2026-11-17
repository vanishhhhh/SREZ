package mailer

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"testing"
	"time"

	"srez/internal/config"
	"srez/internal/security"
)

func TestVerificationMessageAlternativeAndSafeHeaders(t *testing.T) {
	raw := security.Random()
	link := "http://localhost:5174/verify-email#token=" + raw
	data, e := build("SREZ <no-reply@srez.local>", "test@example.com", link)
	if e != nil {
		t.Fatal(e)
	}
	msg, e := mail.ReadMessage(strings.NewReader(string(data)))
	if e != nil {
		t.Fatal(e)
	}
	subject, e := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if e != nil || subject != "Подтвердите email — SREZ" {
		t.Fatal("subject")
	}
	kind, params, e := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if e != nil || kind != "multipart/alternative" {
		t.Fatal("MIME")
	}
	reader := multipart.NewReader(msg.Body, params["boundary"])
	parts := 0
	for {
		p, e := reader.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(p)
		if e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(string(body), link) || !strings.Contains(string(body), "30 минут") {
			t.Fatal("missing verification content")
		}
		if strings.Contains(string(body), "<img") || strings.Contains(string(body), "<script") {
			t.Fatal("external tracking")
		}
		parts++
	}
	if parts != 2 {
		t.Fatal("missing text/html alternative")
	}
}
func TestSMTPTimeoutAndRecipientInjection(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	go func() {
		c, e := listener.Accept()
		if e == nil {
			defer c.Close()
			time.Sleep(250 * time.Millisecond)
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	sender := NewSMTP(config.Mail{Host: "127.0.0.1", Port: strconv.Itoa(port), From: "SREZ <no-reply@srez.local>", TLSMode: "none"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	before := time.Now()
	if e = sender.SendVerification(ctx, "to@example.com", "http://localhost/verify-email#token=x"); e == nil || time.Since(before) > time.Second {
		t.Fatal("SMTP timeout")
	}
	if e = sender.SendVerification(context.Background(), "to@example.com\r\nBcc:other@example.com", "http://localhost"); e == nil {
		t.Fatal("header injection")
	}
}
