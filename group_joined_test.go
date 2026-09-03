package whatsmeow

import (
	"testing"
	"time"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func TestParseGroupCreatePreservesNotificationTimestamp(t *testing.T) {
	const timestamp = int64(1_788_422_400)
	parent := &waBinary.Node{
		Tag: "notification",
		Attrs: waBinary.Attrs{
			"participant": "12345@s.whatsapp.net",
			"t":           "1788422400",
		},
	}
	create := &waBinary.Node{
		Tag:   "create",
		Attrs: waBinary.Attrs{"reason": "invite"},
		Content: []waBinary.Node{{
			Tag: "group",
			Attrs: waBinary.Attrs{
				"id":      "120363407507540222",
				"subject": "Helkyn valmennusryhmä 2026",
			},
		}},
	}

	client := &Client{groupCache: make(map[types.JID]*groupMetaCache)}
	event, _, _, err := client.parseGroupCreate(parent, create)
	if err != nil {
		t.Fatalf("parseGroupCreate failed: %v", err)
	}
	if !event.Timestamp.Equal(time.Unix(timestamp, 0)) {
		t.Fatalf("timestamp = %v, want %v", event.Timestamp, time.Unix(timestamp, 0))
	}
}
