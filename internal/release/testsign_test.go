package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"
)

var (
	testKeyOnce sync.Once
	testPriv    ed25519.PrivateKey
	testPub     ed25519.PublicKey
	testPubB64  string
)

func testSigningKeypair() (ed25519.PublicKey, ed25519.PrivateKey) {
	testKeyOnce.Do(func() {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			panic(err)
		}
		testPub, testPriv = pub, priv
		testPubB64 = base64.StdEncoding.EncodeToString(pub)
	})
	return testPub, testPriv
}

func signManifestJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	_, priv := testSigningKeypair()
	sig := ed25519.Sign(priv, raw)
	env := SignatureEnvelope{
		Algorithm:   SignatureAlgorithmEd25519,
		PublicKeyID: "test-only",
		Signature:   base64.StdEncoding.EncodeToString(sig),
	}
	out, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
