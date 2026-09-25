package storage

import "testing"

func TestReferrerFromZone(t *testing.T) {
	zone := "[ZoneTransfer]\r\nZoneId=3\r\nReferrerUrl=https://site.example/g/535147/\r\nHostUrl=https://cdn.example/f.zip\r\n"
	if got := referrerFromZone([]byte(zone)); got != "https://site.example/g/535147/" {
		t.Fatalf("ReferrerUrl: %q", got)
	}
	for _, z := range []string{
		"[ZoneTransfer]\r\nZoneId=3\r\nHostUrl=https://cdn.example/f.zip\r\n", // без ReferrerUrl
		"[ZoneTransfer]\r\nReferrerUrl=about:blank\r\n",                       // не http(s)
		"",
	} {
		if got := referrerFromZone([]byte(z)); got != "" {
			t.Errorf("%q: ожидалось пусто, получено %q", z, got)
		}
	}
}
