package release

import "testing"

func TestVersionMetadataUnsignedLabBuild(t *testing.T) {
	oldV, oldC, oldT := Version, Commit, TrustedRelease
	Version = "1.0.0-rc1"
	Commit = "abc123def456"
	TrustedRelease = "false"
	defer func() {
		Version, Commit, TrustedRelease = oldV, oldC, oldT
	}()

	info := CurrentInfo()
	if info.TrustedRelease {
		t.Fatal("unsigned lab build must not set trusted_release")
	}
	if !info.Development {
		t.Fatal("unsigned lab build must report development=true")
	}
}

func TestVersionMetadataMaintainerTrustedLabBuild(t *testing.T) {
	oldV, oldC, oldT := Version, Commit, TrustedRelease
	Version = "1.0.0-rc1"
	Commit = "abc123def456"
	TrustedRelease = "true"
	defer func() {
		Version, Commit, TrustedRelease = oldV, oldC, oldT
	}()

	info := CurrentInfo()
	if !info.TrustedRelease {
		t.Fatal("maintainer trusted build must set trusted_release")
	}
	if info.Development {
		t.Fatal("trusted release with pinned version must not be development")
	}
}
