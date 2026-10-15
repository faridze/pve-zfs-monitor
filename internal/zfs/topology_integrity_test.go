package zfs

import (
	"strings"
	"testing"
)

func TestParseAmbiguousDataIndentRejected(t *testing.T) {
	_, err := ParseStatus(loadFixture("ambiguous_data_indent.txt"))
	if err == nil || !strings.Contains(err.Error(), "ambiguous indent") {
		t.Fatalf("expected ambiguous indent error, got %v", err)
	}
}

func TestParseAmbiguousAuxIndentRejected(t *testing.T) {
	_, err := ParseStatus(loadFixture("ambiguous_aux_indent.txt"))
	if err == nil || !strings.Contains(err.Error(), "ambiguous auxiliary indent") {
		t.Fatalf("expected auxiliary indent error, got %v", err)
	}
}

func TestParseMissingParentDepthRejected(t *testing.T) {
	_, err := ParseStatus(loadFixture("missing_parent_depth.txt"))
	if err == nil {
		t.Fatal("expected parse failure for skipped hierarchy level")
	}
}

func TestParseTruncatedBeforeErrorsRejected(t *testing.T) {
	_, err := ParseStatus(loadFixture("truncated_before_errors.txt"))
	if err == nil || !strings.Contains(err.Error(), "missing errors section") {
		t.Fatalf("expected missing errors section, got %v", err)
	}
}

func TestParseHotSpareInUse(t *testing.T) {
	report, err := ParseStatus(loadFixture("auxiliary_spares_inuse.txt"))
	if err != nil {
		t.Fatal(err)
	}
	spares := findChild(report.Pools[0].Root, "spares")
	if spares == nil || len(spares.Children) != 2 {
		t.Fatalf("spares section: %#v", spares)
	}
	inuse := spares.Children[0]
	if inuse.Name != "sdah" || inuse.State != "INUSE" || inuse.StateDetail != "currently in use" {
		t.Fatalf("INUSE spare: %#v", inuse)
	}
	if inuse.ReadErrors != nil || inuse.WriteErrors != nil || inuse.CksumErrors != nil {
		t.Fatalf("INUSE spare counters must be null: %#v", inuse)
	}
	avail := spares.Children[1]
	if avail.State != "AVAIL" || avail.ReadErrors != nil {
		t.Fatalf("AVAIL spare: %#v", avail)
	}
}
