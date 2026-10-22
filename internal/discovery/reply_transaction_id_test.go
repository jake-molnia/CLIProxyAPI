package discovery

import (
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestReplyTransactionIDMatchesOnlyLegacyUnicast(t *testing.T) {
	const queryID = 73
	host := "cpa-regression.local."
	ips := []string{"192.0.2.10", "2001:db8::10"}
	svc := parseMDNSServiceConfig("CPA-Regression", DefaultServiceType, DefaultDomain, 8317, []string{"product=cliproxyapi"})
	for _, builder := range []struct {
		name     string
		question string
		qtype    dnsmessage.Type
		build    func([]byte, bool) ([]byte, bool)
	}{
		{"host", host, dnsmessage.TypeA, func(query []byte, legacy bool) ([]byte, bool) { return buildHostAddressReply(query, host, ips, legacy) }},
		{"service", "_ai-gateway._tcp.local.", dnsmessage.TypePTR, func(query []byte, legacy bool) ([]byte, bool) {
			return buildServiceQueryReply(query, svc, host, ips, legacy)
		}},
	} {
		for _, transport := range []struct {
			name       string
			legacy, qu bool
		}{
			{"multicast", false, false}, {"mdns-unicast", false, true}, {"legacy-unicast", true, false},
		} {
			t.Run(builder.name+"/"+transport.name, func(t *testing.T) {
				name, err := dnsmessage.NewName(builder.question)
				if err != nil {
					t.Fatal(err)
				}
				class := dnsmessage.ClassINET
				if transport.qu {
					class |= dnsmessage.Class(mdnsCacheFlushBit)
				}
				query, err := (&dnsmessage.Message{Header: dnsmessage.Header{ID: queryID}, Questions: []dnsmessage.Question{{Name: name, Type: builder.qtype, Class: class}}}).Pack()
				if err != nil {
					t.Fatal(err)
				}
				reply, unicast := builder.build(query, transport.legacy)
				var response dnsmessage.Message
				if err := response.Unpack(reply); err != nil {
					t.Fatal(err)
				}
				if len(response.Answers) == 0 {
					t.Fatal("expected response records")
				}
				wantID := uint16(0)
				if transport.legacy {
					wantID = queryID
				}
				if response.ID != wantID {
					t.Errorf("response ID = %d, want %d", response.ID, wantID)
				}
				if unicast != (transport.legacy || transport.qu) {
					t.Errorf("unicast = %v", unicast)
				}
			})
		}
	}
}
