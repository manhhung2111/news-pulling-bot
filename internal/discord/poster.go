package discord

import (
	"fmt"
	"log/slog"
)

type Posted struct {
	MessageID      string
	ThreadID       string
	ItemMessageIDs []string // ItemMessageIDs[i] is Digest.Items[i]
}

// PostDigest sends the headline, then (when there are items) a thread with one embed per item seeded with 👍/👎.
func PostDigest(api API, channelID string, r Rendered) (Posted, error) {
	var p Posted
	id, err := api.Send(channelID, r.Headline)
	if err != nil {
		return p, fmt.Errorf("send headline: %w", err)
	}
	p.MessageID = id
	if len(r.Items) == 0 {
		return p, nil
	}
	p.ThreadID, err = api.StartThread(channelID, id, r.ThreadName)
	if err != nil {
		return p, fmt.Errorf("start thread: %w", err)
	}
	for i, e := range r.Items {
		mid, err := api.SendEmbed(p.ThreadID, e)
		if err != nil {
			return p, fmt.Errorf("send item %d: %w", i+1, err)
		}
		p.ItemMessageIDs = append(p.ItemMessageIDs, mid)
		for _, emoji := range []string{"👍", "👎"} {
			if err := api.React(p.ThreadID, mid, emoji); err != nil {
				slog.Warn("seed reaction failed", "item", i+1, "emoji", emoji, "err", err)
			}
		}
	}
	return p, nil
}

// PostFailure posts the one-line failure message.
func PostFailure(api API, channelID, reason string) error {
	_, err := api.Send(channelID, RenderFailure(reason))
	return err
}
