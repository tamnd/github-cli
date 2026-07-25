package gitproto

import "testing"

// pkt builds a pkt-line stream from payloads. A "" payload is a flush packet.
func pkt(payloads ...string) []byte {
	var b []byte
	for _, p := range payloads {
		if p == "" {
			b = append(b, "0000"...)
			continue
		}
		n := len(p) + 4
		const hex = "0123456789abcdef"
		b = append(b, hex[n>>12&0xf], hex[n>>8&0xf], hex[n>>4&0xf], hex[n&0xf])
		b = append(b, p...)
	}
	return b
}

const (
	shaHead = "1111111111111111111111111111111111111111"
	shaMain = "2222222222222222222222222222222222222222"
	shaTag  = "3333333333333333333333333333333333333333"
	shaPeel = "4444444444444444444444444444444444444444"
	shaPull = "5555555555555555555555555555555555555555"
)

func TestParse(t *testing.T) {
	body := pkt(
		"# service=git-upload-pack\n",
		"",
		shaHead+" HEAD\x00multi_ack symref=HEAD:refs/heads/trunk object-format=sha1\n",
		shaMain+" refs/heads/trunk\n",
		shaTag+" refs/tags/v1.0.0\n",
		shaPeel+" refs/tags/v1.0.0^{}\n",
		shaPull+" refs/pull/42/head\n",
		"",
	)
	ad, err := Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	if ad.Head != shaHead {
		t.Errorf("head %q", ad.Head)
	}
	if ad.DefaultBranch != "trunk" {
		t.Errorf("default branch %q", ad.DefaultBranch)
	}
	if len(ad.Refs) != 3 {
		t.Fatalf("refs %d, the peeled entry should have folded into its tag", len(ad.Refs))
	}
	branches := ad.Branches()
	if len(branches) != 1 || branches[0].Name != "trunk" || branches[0].SHA != shaMain {
		t.Errorf("branches %+v", branches)
	}
	tags := ad.Tags()
	if len(tags) != 1 || tags[0].Name != "v1.0.0" {
		t.Fatalf("tags %+v", tags)
	}
	// An annotated tag keeps both: the tag object and the commit it points at.
	if tags[0].SHA != shaTag || tags[0].Peeled != shaPeel {
		t.Errorf("tag sha %q peeled %q", tags[0].SHA, tags[0].Peeled)
	}
	pulls := ad.PullHeads()
	if len(pulls) != 1 || pulls[0].Name != "42/head" {
		t.Errorf("pull heads %+v", pulls)
	}
}

func TestParseRejectsHTML(t *testing.T) {
	// A private or missing repository answers with a page, and a parser that
	// shrugs at that hands back an empty ref list that looks like a real answer.
	for _, body := range []string{
		"<!DOCTYPE html>\n<html><head><title>GitHub</title></head></html>",
		"",
		"00x4bad length",
		"0005",
	} {
		if _, err := Parse([]byte(body)); err == nil {
			t.Errorf("accepted %q", body[:min(len(body), 20)])
		}
	}
}
