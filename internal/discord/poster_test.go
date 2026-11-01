package discord

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// fakeAPI records calls and serves canned reads.
type fakeAPI struct {
	calls     []string
	n         int
	reactErr  error
	reactions map[string][]string // "msgID emoji" -> user IDs
	messages  map[string][]Message
}

func (f *fakeAPI) id() string { f.n++; return fmt.Sprintf("m%d", f.n) }

func (f *fakeAPI) Send(ch, content string) (string, error) {
	f.calls = append(f.calls, "Send "+ch+" "+content)
	return f.id(), nil
}
func (f *fakeAPI) StartThread(ch, msg, name string) (string, error) {
	f.calls = append(f.calls, "StartThread "+ch+" "+msg+" "+name)
	return "thread1", nil
}
func (f *fakeAPI) SendEmbed(ch string, e *discordgo.MessageEmbed) (string, error) {
	f.calls = append(f.calls, "SendEmbed "+ch+" "+e.Title)
	return f.id(), nil
}
func (f *fakeAPI) React(ch, msg, emoji string) error {
	f.calls = append(f.calls, "React "+msg+" "+emoji)
	return f.reactErr
}
func (f *fakeAPI) Reactions(ch, msg, emoji string) ([]string, error) {
	return f.reactions[msg+" "+emoji], nil
}
func (f *fakeAPI) Messages(ch string, limit int) ([]Message, error) {
	return f.messages[ch], nil
}

func rendered(n int) Rendered {
	r := Rendered{Headline: "head", ThreadName: "Digest — 2026-11-01"}
	for i := 0; i < n; i++ {
		r.Items = append(r.Items, &discordgo.MessageEmbed{Title: fmt.Sprintf("T%d", i)})
	}
	return r
}

func TestPostDigestOrderOfCalls(t *testing.T) {
	api := &fakeAPI{}
	p, err := PostDigest(api, "chan", rendered(2))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Send chan head", "StartThread chan m1 Digest — 2026-11-01",
		"SendEmbed thread1 T0", "React m2 👍", "React m2 👎",
		"SendEmbed thread1 T1", "React m3 👍", "React m3 👎",
	}
	if strings.Join(api.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls =\n%s", strings.Join(api.calls, "\n"))
	}
	if p.MessageID != "m1" || p.ThreadID != "thread1" || strings.Join(p.ItemMessageIDs, ",") != "m2,m3" {
		t.Errorf("posted = %+v", p)
	}
}

func TestPostDigestZeroItemsNoThread(t *testing.T) {
	api := &fakeAPI{}
	p, err := PostDigest(api, "chan", rendered(0))
	if err != nil || len(api.calls) != 1 || p.ThreadID != "" {
		t.Fatalf("calls=%v posted=%+v err=%v", api.calls, p, err)
	}
}

func TestPostDigestReactionFailureIgnored(t *testing.T) {
	api := &fakeAPI{reactErr: errors.New("missing permission")}
	p, err := PostDigest(api, "chan", rendered(1))
	if err != nil || len(p.ItemMessageIDs) != 1 {
		t.Fatalf("posted=%+v err=%v", p, err)
	}
}

func TestPostFailureSendsRenderedText(t *testing.T) {
	api := &fakeAPI{}
	if err := PostFailure(api, "chan", "boom"); err != nil {
		t.Fatal(err)
	}
	if api.calls[0] != "Send chan "+RenderFailure("boom") {
		t.Fatalf("calls = %v", api.calls)
	}
}

