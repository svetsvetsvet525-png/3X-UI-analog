package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestBuildOnlineClientViews_LiveLocalIPsWin(t *testing.T) {
	views := buildOnlineClientViews(
		map[string][]string{"local": {"alice@x", "bob@x"}},
		map[string][]OnlineClientIP{
			"alice@x": {{IP: "1.1.1.1", Timestamp: 300}, {IP: "2.2.2.2", Timestamp: 100}},
		},
		map[string]map[string][]model.ClientIpEntry{},
		map[string]string{},
		"local",
		map[string]clientInboundMeta{},
	)
	if len(views) != 2 {
		t.Fatalf("want 2 rows, got %d: %v", len(views), views)
	}
	alice := views[0]
	if alice.Node != "" || alice.Email != "alice@x" {
		t.Fatalf("local alice row wrong: %+v", alice)
	}
	if len(alice.IPs) != 2 || alice.IPs[0].IP != "1.1.1.1" {
		t.Fatalf("live IPs should be newest-first, got %+v", alice.IPs)
	}
}

func TestBuildOnlineClientViews_RemoteNodeFromAttribution(t *testing.T) {
	views := buildOnlineClientViews(
		map[string][]string{"guid-A": {"carol@x"}, "guid-B": {"carol@x"}},
		map[string][]OnlineClientIP{},
		map[string]map[string][]model.ClientIpEntry{
			"guid-A": {"carol@x": {{IP: "9.9.9.9", Timestamp: 5}}},
			"guid-B": {"carol@x": {{IP: "8.8.8.8", Timestamp: 9}}},
		},
		map[string]string{"guid-A": "edge-1"},
		"local",
		map[string]clientInboundMeta{},
	)
	if len(views) != 2 {
		t.Fatalf("want 2 rows (one per node), got %d: %v", len(views), views)
	}
	// Sorted by node name: "" (guid-B has no known name) before "edge-1".
	if views[0].Node != "" || views[0].IPs[0].IP != "8.8.8.8" {
		t.Fatalf("unknown-guid row should be anonymous: %+v", views[0])
	}
	if views[1].Node != "edge-1" || views[1].IPs[0].IP != "9.9.9.9" {
		t.Fatalf("node-labelled row wrong: %+v", views[1])
	}
}

func TestLoadClientInboundMetas_PrimaryInbound(t *testing.T) {
	setupClientIpTestDB(t)
	db := database.GetDB()

	if err := db.Create(&model.Inbound{Remark: "VLESS-TLS", Tag: "in-1", Protocol: "vless", Port: 443}).Error; err != nil {
		t.Fatalf("inbound 1: %v", err)
	}
	if err := db.Create(&model.Inbound{Remark: "HY2", Tag: "in-2", Protocol: "hysteria", Port: 8443}).Error; err != nil {
		t.Fatalf("inbound 2: %v", err)
	}
	rec := &model.ClientRecord{Email: "alice@x", Comment: "Alice phone"}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("client: %v", err)
	}
	// Attach to the higher-id inbound first; the meta must pick the lowest id.
	if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: 2}).Error; err != nil {
		t.Fatalf("attach 2: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: 1}).Error; err != nil {
		t.Fatalf("attach 1: %v", err)
	}

	meta := (&InboundService{}).loadClientInboundMetas([]string{"alice@x", "ghost@x"})
	m, ok := meta["alice@x"]
	if !ok {
		t.Fatalf("alice meta missing: %v", meta)
	}
	if m.Remark != "Alice phone" || m.Inbound != "VLESS-TLS" || m.Protocol != "vless" {
		t.Fatalf("metas wrong: %+v", m)
	}
	if _, ok := meta["ghost@x"]; ok {
		t.Fatalf("ghost client should be absent: %v", meta)
	}
}