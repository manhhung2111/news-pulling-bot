package discord

import (
	"time"

	"github.com/bwmarrin/discordgo"
)

// API is the slice of Discord the digest uses. REST only; no gateway connection.
type API interface {
	Send(channelID, content string) (msgID string, err error)
	StartThread(channelID, msgID, name string) (threadID string, err error)
	SendEmbed(channelID string, e *discordgo.MessageEmbed) (msgID string, err error)
	React(channelID, msgID, emoji string) error
	Reactions(channelID, msgID, emoji string) (userIDs []string, err error)
	Messages(channelID string, limit int) ([]Message, error)
}

type Message struct {
	ID        string
	AuthorID  string
	Content   string
	ReplyToID string
	Timestamp time.Time
}

type session struct{ s *discordgo.Session }

// NewAPI returns an API backed by a bot token.
func NewAPI(token string) (API, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}
	return &session{s: s}, nil
}

func (a *session) Send(channelID, content string) (string, error) {
	m, err := a.s.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content: content,
		Flags:   discordgo.MessageFlagsSuppressEmbeds,
	})
	if err != nil {
		return "", err
	}
	return m.ID, nil
}

func (a *session) StartThread(channelID, msgID, name string) (string, error) {
	ch, err := a.s.MessageThreadStartComplex(channelID, msgID, &discordgo.ThreadStart{Name: name, AutoArchiveDuration: 1440})
	if err != nil {
		return "", err
	}
	return ch.ID, nil
}

func (a *session) SendEmbed(channelID string, e *discordgo.MessageEmbed) (string, error) {
	m, err := a.s.ChannelMessageSendEmbed(channelID, e)
	if err != nil {
		return "", err
	}
	return m.ID, nil
}

func (a *session) React(channelID, msgID, emoji string) error {
	return a.s.MessageReactionAdd(channelID, msgID, emoji)
}

func (a *session) Reactions(channelID, msgID, emoji string) ([]string, error) {
	users, err := a.s.MessageReactions(channelID, msgID, emoji, 100, "", "")
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(users))
	for i, u := range users {
		ids[i] = u.ID
	}
	return ids, nil
}

func (a *session) Messages(channelID string, limit int) ([]Message, error) {
	ms, err := a.s.ChannelMessages(channelID, limit, "", "", "")
	if err != nil {
		return nil, err
	}
	out := make([]Message, 0, len(ms))
	for _, m := range ms {
		msg := Message{ID: m.ID, Content: m.Content, Timestamp: m.Timestamp}
		if m.Author != nil {
			msg.AuthorID = m.Author.ID
		}
		if m.MessageReference != nil {
			msg.ReplyToID = m.MessageReference.MessageID
		}
		out = append(out, msg)
	}
	return out, nil
}
