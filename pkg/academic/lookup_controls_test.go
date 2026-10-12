package academic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"websearch/pkg/antirobot"
)

func TestLookupDOISourceControls(t *testing.T) {
	for _, tc := range []struct{ crossref, openalex bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		t.Run(fmt.Sprintf("crossref=%v/openalex=%v", tc.crossref, tc.openalex), func(t *testing.T) {
			var crossrefHits, openalexHits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/works/") {
					openalexHits.Add(1)
				} else {
					crossrefHits.Add(1)
				}
				fmt.Fprint(w, `{}`)
			}))
			defer server.Close()
			oldAPI, oldCrossref, oldClient := openalexAPI, crossrefPath, lookupClient
			openalexAPI, crossrefPath, lookupClient = server.URL, server.URL+"/crossref", server.Client()
			t.Cleanup(func() { openalexAPI, crossrefPath, lookupClient = oldAPI, oldCrossref, oldClient })
			LookupDOI(context.Background(), "10.1234/test", antirobot.CrossrefOpts{Enabled: tc.crossref}, antirobot.OpenAlexOpts{Enabled: tc.openalex})
			if (crossrefHits.Load() == 1) != tc.crossref || (openalexHits.Load() == 1) != tc.openalex {
				t.Fatalf("exact lookup violated controls: crossref=%d openalex=%d", crossrefHits.Load(), openalexHits.Load())
			}
		})
	}
}
