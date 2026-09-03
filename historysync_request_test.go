// Copyright (c) 2026 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"encoding/hex"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestBuildFullHistorySyncRequest(t *testing.T) {
	originalProps := store.DeviceProps
	t.Cleanup(func() { store.DeviceProps = originalProps })
	store.DeviceProps = &waCompanionReg.DeviceProps{
		HistorySyncConfig: &waCompanionReg.DeviceProps_HistorySyncConfig{
			FullSyncDaysLimit:         proto.Uint32(3650),
			FullSyncSizeMbLimit:       proto.Uint32(2048),
			OnDemandReady:             proto.Bool(true),
			CompleteOnDemandReady:     proto.Bool(true),
			SupportGroupHistory:       proto.Bool(true),
			SupportMessageAssociation: proto.Bool(true),
		},
	}

	from := time.Unix(1_785_960_000, 0)
	msg := (&Client{}).BuildFullHistorySyncRequest(from, 30)
	protocolMsg := msg.GetProtocolMessage()
	if protocolMsg.GetType() != waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE {
		t.Fatalf("unexpected protocol message type: %s", protocolMsg.GetType())
	}
	request := protocolMsg.GetPeerDataOperationRequestMessage()
	if request.GetPeerDataOperationRequestType() != waE2E.PeerDataOperationRequestType_FULL_HISTORY_SYNC_ON_DEMAND {
		t.Fatalf("unexpected peer request type: %s", request.GetPeerDataOperationRequestType())
	}
	fullRequest := request.GetFullHistorySyncOnDemandRequest()
	if fullRequest == nil {
		t.Fatal("full history sync request is nil")
	}
	if !proto.Equal(fullRequest.GetHistorySyncConfig(), store.DeviceProps.HistorySyncConfig) {
		t.Fatalf("history sync config was not copied into request: %v", fullRequest.GetHistorySyncConfig())
	}
	if fullRequest.HistorySyncConfig == store.DeviceProps.HistorySyncConfig {
		t.Fatal("history sync config must be cloned")
	}
	config := fullRequest.GetFullHistorySyncOnDemandConfig()
	if config.GetHistoryFromTimestamp() != uint64(from.Unix()) {
		t.Errorf("unexpected history-from timestamp: %d", config.GetHistoryFromTimestamp())
	}
	if config.GetHistoryDurationDays() != 30 {
		t.Errorf("unexpected history duration: %d", config.GetHistoryDurationDays())
	}
	requestID := fullRequest.GetRequestMetadata().GetRequestID()
	if decoded, err := hex.DecodeString(requestID); err != nil || len(decoded) != 16 {
		t.Errorf("request ID is not 16 random bytes encoded as hex: %q (%v)", requestID, err)
	}
}

func TestBuildFullHistorySyncRequestAllowsNilHistorySyncConfig(t *testing.T) {
	originalProps := store.DeviceProps
	t.Cleanup(func() { store.DeviceProps = originalProps })
	store.DeviceProps = &waCompanionReg.DeviceProps{}

	request := (&Client{}).BuildFullHistorySyncRequest(time.Unix(1, 0), 1).
		GetProtocolMessage().GetPeerDataOperationRequestMessage().GetFullHistorySyncOnDemandRequest()
	if request.GetHistorySyncConfig() != nil {
		t.Fatalf("expected nil history sync config, got %v", request.GetHistorySyncConfig())
	}
}

func TestBuildHistorySyncRequestUsesMillisecondsAndInlineResponse(t *testing.T) {
	anchorTime := time.Unix(1_788_422_400, 321_000_000)
	request := (&Client{}).BuildHistorySyncRequest(&types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     types.JID{User: "358442506575", Server: types.DefaultUserServer},
			IsFromMe: true,
		},
		ID:        "anchor-id",
		Timestamp: anchorTime,
	}, 50).GetProtocolMessage().GetPeerDataOperationRequestMessage().GetHistorySyncOnDemandRequest()

	if request.GetOldestMsgTimestampMS() != anchorTime.UnixMilli() {
		t.Fatalf("oldest timestamp = %d, want milliseconds %d", request.GetOldestMsgTimestampMS(), anchorTime.UnixMilli())
	}
	if !request.GetSupportInlineResponse() {
		t.Fatal("supportInlineResponse must be enabled")
	}
}

func TestBuildHistorySyncRequestAllowsEmptyAnchorForRecentMessages(t *testing.T) {
	request := (&Client{}).BuildHistorySyncRequest(&types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat: types.JID{User: "358442506575", Server: types.DefaultUserServer},
		},
	}, 50).GetProtocolMessage().GetPeerDataOperationRequestMessage().GetHistorySyncOnDemandRequest()

	if request.OldestMsgID != nil || request.OldestMsgFromMe != nil || request.OldestMsgTimestampMS != nil {
		t.Fatalf("empty anchor fields must stay absent: %+v", request)
	}
	if !request.GetSupportInlineResponse() {
		t.Fatal("supportInlineResponse must be enabled")
	}
}

func TestHistorySyncPeerMessagesForceDeliveryToPrimary(t *testing.T) {
	for _, requestType := range []waE2E.PeerDataOperationRequestType{
		waE2E.PeerDataOperationRequestType_HISTORY_SYNC_ON_DEMAND,
		waE2E.PeerDataOperationRequestType_FULL_HISTORY_SYNC_ON_DEMAND,
	} {
		t.Run(requestType.String(), func(t *testing.T) {
			message := &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
				Type: waE2E.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
				PeerDataOperationRequestMessage: &waE2E.PeerDataOperationRequestMessage{
					PeerDataOperationRequestType: requestType.Enum(),
				},
			}}
			attrs := peerMessageDeliveryAttrs(message)
			if attrs["push_priority"] != "high_force" {
				t.Errorf("push_priority = %v, want high_force", attrs["push_priority"])
			}
			if attrs["privacy_sensitive"] != "1" {
				t.Errorf("privacy_sensitive = %v, want 1", attrs["privacy_sensitive"])
			}
		})
	}
}

func TestFullHistorySyncResponseIsDispatched(t *testing.T) {
	client := &Client{}
	var got *events.FullHistorySyncResponse
	client.AddEventHandler(func(evt any) {
		got, _ = evt.(*events.FullHistorySyncResponse)
	})
	responseCode := waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult_DECLINED_SHARING_HISTORY
	requestID := "request-123"
	client.handleProtocolMessage(t.Context(), &types.MessageInfo{
		MessageSource: types.MessageSource{
			IsFromMe: true,
			Sender:   types.JID{User: "12345", Server: types.DefaultUserServer},
		},
	}, &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		PeerDataOperationRequestResponseMessage: &waE2E.PeerDataOperationRequestResponseMessage{
			PeerDataOperationRequestType: waE2E.PeerDataOperationRequestType_FULL_HISTORY_SYNC_ON_DEMAND.Enum(),
			StanzaID:                     proto.String("stanza-123"),
			PeerDataOperationResult: []*waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult{{
				FullHistorySyncOnDemandRequestResponse: &waE2E.PeerDataOperationRequestResponseMessage_PeerDataOperationResult_FullHistorySyncOnDemandRequestResponse{
					RequestMetadata: &waE2E.FullHistorySyncOnDemandRequestMetadata{RequestID: &requestID},
					ResponseCode:    &responseCode,
				},
			}},
		},
	}})
	if got == nil {
		t.Fatal("expected FullHistorySyncResponse event")
	}
	if got.StanzaID != "stanza-123" || got.RequestID != requestID || got.ResponseCode != responseCode {
		t.Fatalf("unexpected response event: %+v", got)
	}
}
