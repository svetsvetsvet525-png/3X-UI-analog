package service

import (
	"sort"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// OnlineClientIP is one source IP shown on the Online Clients page.
type OnlineClientIP struct {
	IP        string `json:"ip" example:"1.2.3.4"`
	Timestamp int64  `json:"timestamp" example:"1700000000"`
}

// OnlineClientView is one row of the Online Clients page: a client with a live
// connection (or one observed connecting recently) together with the source
// IPs of that traffic and the node/inbound the traffic goes through. Node is
// "" for clients on this local panel, and IPs are the live source addresses
// where the local core exposes them, otherwise the per-node attribution table.
type OnlineClientView struct {
	Email    string           `json:"email" example:"user@example.com"`
	Remark   string           `json:"remark" example:"Alice's phone"` // client comment
	Inbound  string           `json:"inbound" example:"VLESS-TLS"`    // client's primary inbound remark
	Protocol string           `json:"protocol" example:"vless"`       // ... and protocol
	Node     string           `json:"node" example:"edge-1"`          // "" = this local panel
	IPs      []OnlineClientIP `json:"ips" example:"[{\"ip\":\"1.2.3.4\",\"timestamp\":1700000000}]"`
}

// clientInboundMeta is the per-client display metadata resolved from the
// normalized clients / client_inbounds relation (primary inbound = lowest id).
type clientInboundMeta struct {
	Remark   string
	Inbound  string
	Protocol string
}

// GetOnlineClientsView builds the Online Clients page payload. The online set
// comes from GetOnlineClientsByGuid (this panel's own clients plus every node's
// subtree); client IPs come from the live online-stats API for the local panel
// and from the per-node NodeClientIp attribution table everywhere else.
func (s *InboundService) GetOnlineClientsView() []OnlineClientView {
	onlineByGuid := s.GetOnlineClientsByGuid()
	if len(onlineByGuid) == 0 {
		return nil
	}

	// Local source IPs, straight from the running core; includes clients whose
	// byte-delta is zero but who hold a live connection.
	liveLocal := make(map[string][]OnlineClientIP)
	if users, ok, err := (&XrayService{}).GetOnlineUsers(); err == nil && ok {
		for _, u := range users {
			if u.Email == "" || len(u.IPs) == 0 {
				continue
			}
			ips := make([]OnlineClientIP, 0, len(u.IPs))
			for _, e := range u.IPs {
				ips = append(ips, OnlineClientIP{IP: e.IP, Timestamp: e.LastSeen})
			}
			liveLocal[u.Email] = ips
		}
	}

	emails := make(map[string]struct{}, len(liveLocal))
	for _, list := range onlineByGuid {
		for _, email := range list {
			emails[email] = struct{}{}
		}
	}
	for email := range liveLocal {
		emails[email] = struct{}{}
	}
	emailList := make([]string, 0, len(emails))
	for email := range emails {
		if email != "" {
			emailList = append(emailList, email)
		}
	}

	attrByGuidEmail, _ := s.GetClientIpsByGuid()
	views := buildOnlineClientViews(
		onlineByGuid,
		liveLocal,
		attrByGuidEmail,
		s.nodeGuidNameMap(),
		s.panelGuid(),
		s.loadClientInboundMetas(emailList),
	)
	return views
}

// buildOnlineClientViews merges the live local IPs with per-node attribution
// rows into one row per (node, email), newest IP first. Pure so the merge and
// node-labelling rules are unit-testable without a running core.
func buildOnlineClientViews(
	onlineByGuid map[string][]string,
	liveLocal map[string][]OnlineClientIP,
	attrByGuidEmail map[string]map[string][]model.ClientIpEntry,
	guidName map[string]string,
	localGuid string,
	meta map[string]clientInboundMeta,
) []OnlineClientView {
	var out []OnlineClientView
	for guid, list := range onlineByGuid {
		node := ""
		if guid != "" && guid != localGuid {
			if name, ok := guidName[guid]; ok {
				node = name
			}
		}
		for _, email := range list {
			if email == "" {
				continue
			}
			ips := make([]OnlineClientIP, 0, len(liveLocal[email]))
			if guid == localGuid {
				// Live core IPs win; a delta-online local client falls back to the
				// attribution rows the ip scan recorded for the local guid.
				ips = append(ips, liveLocal[email]...)
			}
			if len(ips) == 0 {
				if entries, ok := attrByGuidEmail[guid][email]; ok {
					for _, e := range entries {
						if e.IP != "" {
							ips = append(ips, OnlineClientIP{IP: e.IP, Timestamp: e.Timestamp})
						}
					}
				}
			}
			sort.Slice(ips, func(i, j int) bool { return ips[i].Timestamp > ips[j].Timestamp })
			m := meta[email]
			out = append(out, OnlineClientView{
				Email:    email,
				Remark:   m.Remark,
				Inbound:  m.Inbound,
				Protocol: m.Protocol,
				Node:     node,
				IPs:      ips,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Node != out[j].Node {
			return out[i].Node < out[j].Node
		}
		return out[i].Email < out[j].Email
	})
	return out
}

// loadClientInboundMetas resolves, per email, the client comment and the primary
// inbound (lowest client_inbounds.inbound_id) remark + protocol in chunked
// queries instead of one settings-LIKE scan per email.
func (s *InboundService) loadClientInboundMetas(emails []string) map[string]clientInboundMeta {
	meta := make(map[string]clientInboundMeta, len(emails))
	if len(emails) == 0 {
		return meta
	}
	db := database.GetDB()

	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var rows []struct {
			Email   string
			Comment string
		}
		if err := db.Model(&model.ClientRecord{}).
			Select("email, comment").
			Where("email IN ?", batch).
			Scan(&rows).Error; err != nil {
			continue
		}
		for _, r := range rows {
			if cur, ok := meta[r.Email]; ok {
				cur.Remark = r.Comment
				meta[r.Email] = cur
			} else {
				meta[r.Email] = clientInboundMeta{Remark: r.Comment}
			}
		}
	}

	minInbound := make(map[string]int, len(emails))
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var pairs []struct {
			Email     string
			InboundId int
		}
		if err := db.Table("client_inbounds").
			Select("clients.email AS email, client_inbounds.inbound_id AS inbound_id").
			Joins("JOIN clients ON clients.id = client_inbounds.client_id").
			Where("clients.email IN ?", batch).
			Scan(&pairs).Error; err != nil {
			continue
		}
		for _, p := range pairs {
			if cur, ok := minInbound[p.Email]; !ok || p.InboundId < cur {
				minInbound[p.Email] = p.InboundId
			}
		}
	}
	if len(minInbound) == 0 {
		return meta
	}

	idSet := make(map[int]struct{}, len(minInbound))
	ids := make([]int, 0, len(minInbound))
	for _, id := range minInbound {
		if _, seen := idSet[id]; seen {
			continue
		}
		idSet[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Ints(ids)

	inboundByID := make(map[int]model.Inbound, len(ids))
	for lo := 0; lo < len(ids); lo += sqlInChunk {
		hi := min(lo+sqlInChunk, len(ids))
		var page []model.Inbound
		if err := db.Model(&model.Inbound{}).Where("id IN ?", ids[lo:hi]).Find(&page).Error; err != nil {
			continue
		}
		for _, ib := range page {
			inboundByID[ib.Id] = ib
		}
	}

	for email, id := range minInbound {
		ib, ok := inboundByID[id]
		if !ok {
			continue
		}
		cur := meta[email]
		cur.Inbound = ib.Remark
		cur.Protocol = string(ib.Protocol)
		meta[email] = cur
	}
	return meta
}