// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"compress/zlib"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waGroupHistory"
	"go.mau.fi/whatsmeow/proto/waWeb"
)

func TestDecodeGroupHistory(t *testing.T) {
	original := &waGroupHistory.GroupHistory{Messages: []*waWeb.WebMessageInfo{{
		Key:     &waCommon.MessageKey{ID: proto.String("message-1")},
		Message: &waE2E.Message{Conversation: proto.String("shared history")},
	}}}
	raw, err := proto.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	if _, err = writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	decoded, err := decodeGroupHistory(compressed.Bytes())
	if err != nil {
		t.Fatalf("decodeGroupHistory failed: %v", err)
	}
	if !proto.Equal(decoded, original) {
		t.Fatalf("decoded group history differs: %v", decoded)
	}
	if GetMediaType(&waE2E.MessageHistoryBundle{}) != MediaHistory {
		t.Fatal("message history bundle must use WhatsApp History media keys")
	}
}
