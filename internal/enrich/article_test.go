package enrich

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestArticleTextExtractsBody(t *testing.T) {
	para := strings.Repeat("Keep a decisions log that the agent reads at the start of every session. ", 20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "ua" {
			t.Errorf("UA = %q", r.UserAgent())
		}
		fmt.Fprintf(w, `<html><head><title>Post</title></head><body>
		<nav><a href="/">Home</a> NAVIGATION-MENU <a href="/about">About</a></nav>
		<article><h1>Context engineering</h1><p>%s</p><p>%s</p></article>
		<footer>FOOTER-LINKS</footer></body></html>`, para, para)
	}))
	defer srv.Close()
	got, err := ArticleText(context.Background(), srv.Client(), "ua", srv.URL+"/post")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "decisions log") || strings.Contains(got, "NAVIGATION-MENU") {
		t.Fatalf("got %q", got[:min(len(got), 200)])
	}
}

func TestArticleTextCapped(t *testing.T) {
	para := strings.Repeat("word ", 5000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<html><body><article><p>%s</p></article></body></html>`, para)
	}))
	defer srv.Close()
	got, err := ArticleText(context.Background(), srv.Client(), "", srv.URL)
	if err != nil || len([]rune(got)) > 8000 {
		t.Fatalf("len=%d err=%v", len([]rune(got)), err)
	}
}

func TestArticleTextHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer srv.Close()
	if _, err := ArticleText(context.Background(), srv.Client(), "", srv.URL); err == nil {
		t.Fatal("expected error")
	}
}
