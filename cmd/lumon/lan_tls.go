package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Every LAN game runs inside TLS 1.3. There is no authority to vouch for a host,
// so each host keeps one key, and a joiner trusts it the way ssh trusts a server:
// the first time, the player compares the key's fingerprint with the one on the
// host's screen, and from then on it must not change for that address.
//
// The key and the trusted fingerprints live in lanConfigDir, outside any game's
// files, readable by this user only.

const (
	lanKeyFile   = "host.key"
	lanKnownFile = "known_hosts"

	// lanKnownLimit bounds the trusted-hosts file read back, and lanShownHex is
	// how much of a fingerprint is shown: 80 bits, too many to forge a key for.
	lanKnownLimit = 256 << 10
	lanShownHex   = 20
)

// lanConfigDir is where the host key and trusted fingerprints are kept. Tests
// point it elsewhere.
var lanConfigDir = func() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("no place to keep LAN keys: %w", err)
	}
	return filepath.Join(dir, "lumon", "lan"), nil
}

// lanIdentity is a host's key, as a certificate TLS can present, and its
// fingerprint.
type lanIdentity struct {
	cert tls.Certificate
	key  string
}

// lanFingerprint is the SHA-256 of a certificate's public key, in hex. It names
// the key, so it stays the same when the certificate is remade.
func lanFingerprint(c *x509.Certificate) string {
	sum := sha256.Sum256(c.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:])
}

// lanShowKey is how a fingerprint is shown to people: its start, in groups of
// four, such as 4F2A 91C3 0B7E 55D2 A8F1.
func lanShowKey(key string) string {
	key = strings.ToUpper(key[:min(len(key), lanShownHex)])
	var b strings.Builder
	for i := 0; i < len(key); i += 4 {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(key[i:min(i+4, len(key))])
	}
	return b.String()
}

// lanPrivateDir makes the directory if need be and sees that only this user can
// use it.
func lanPrivateDir() (string, error) {
	dir, err := lanConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("could not make %s: %w", dir, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil {
			return "", fmt.Errorf("could not protect %s: %w", dir, err)
		}
	}
	return dir, nil
}

// lanPrivateFile reports why a file is no fit to hold a secret: not a plain file,
// or open to other users.
func lanPrivateFile(path string, info fs.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a plain file", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s can be read by other users: run chmod 600 on it", path)
	}
	return nil
}

// loadLANIdentity reads this user's host key, making one the first time.
func loadLANIdentity() (*lanIdentity, error) {
	dir, err := lanPrivateDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, lanKeyFile)
	priv, err := readLANKey(path)
	if errors.Is(err, fs.ErrNotExist) {
		priv, err = makeLANKey(path)
	}
	if err != nil {
		return nil, err
	}
	return lanIdentityFor(priv)
}

func readLANKey(path string) (ed25519.PrivateKey, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if err := lanPrivateFile(path, info); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("%s is not a lumon host key", path)
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	priv, ok := k.(ed25519.PrivateKey)
	if err != nil || !ok {
		return nil, fmt.Errorf("%s is not a lumon host key", path)
	}
	return priv, nil
}

// makeLANKey writes a new key. If another lumon wrote one first, that one is
// used.
func makeLANKey(path string) (ed25519.PrivateKey, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return readLANKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("could not save a host key: %w", err)
	}
	err = pem.Encode(f, &pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("could not save a host key: %w", err)
	}
	return priv, nil
}

// lanIdentityFor makes the certificate a key is presented in. Nobody checks its
// names or dates: a joiner trusts the key itself.
func lanIdentityFor(priv ed25519.PrivateKey) (*lanIdentity, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "lumon LAN host"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, priv.Public(), priv)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &lanIdentity{
		cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv, Leaf: leaf},
		key:  lanFingerprint(leaf),
	}, nil
}

// serverConfig is the host's side of every connection.
func (id *lanIdentity) serverConfig() *tls.Config {
	return &tls.Config{
		MinVersion:             tls.VersionTLS13,
		Certificates:           []tls.Certificate{id.cert},
		SessionTicketsDisabled: true,
	}
}

// lanKnown is the fingerprints this user has trusted, by the address dialled.
type lanKnown map[string]string

// lanKnownMu keeps two joins in this process from writing the file at once.
var lanKnownMu sync.Mutex

// loadLANKnown reads the trusted fingerprints. None yet is an empty list.
func loadLANKnown() (lanKnown, error) {
	dir, err := lanPrivateDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, lanKnownFile)
	known := lanKnown{}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return known, nil
	}
	if err != nil {
		return nil, err
	}
	if err := lanPrivateFile(path, info); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(io.LimitReader(f, lanKnownLimit))
	for sc.Scan() {
		addr, key, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if ok && addr != "" && len(key) == sha256.Size*2 {
			if _, err := hex.DecodeString(key); err == nil {
				known[addr] = key
			}
		}
	}
	return known, sc.Err()
}

// trust records a fingerprint for an address, replacing any it had, and saves
// the list. The file is replaced whole so a crash never leaves half of it.
func (k lanKnown) trust(addr, key string) error {
	lanKnownMu.Lock()
	defer lanKnownMu.Unlock()
	dir, err := lanPrivateDir()
	if err != nil {
		return err
	}
	k[addr] = key
	var b bytes.Buffer
	for a, fp := range k {
		fmt.Fprintf(&b, "%s %s\n", a, fp)
	}
	tmp, err := os.CreateTemp(dir, lanKnownFile+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		_ = tmp.Close()
		return err
	}
	_, err = tmp.Write(b.Bytes())
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, lanKnownFile))
}

// lanDoubt is a host whose key this user has not trusted for its address: new,
// or different from the one trusted before (was).
type lanDoubt struct {
	addr, key, was string
}

func (d *lanDoubt) Error() string {
	if d.was != "" {
		return "the key of " + d.addr + " has changed"
	}
	return "this is the first time joining " + d.addr
}

// clientConfig checks the host's key against what this user trusts for addr.
// TLS itself proves the host holds that key; the chain of names and dates that
// InsecureSkipVerify leaves out means nothing for a key nobody vouched for.
func (k lanKnown) clientConfig(addr string) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("the host showed no key")
			}
			key := lanFingerprint(cs.PeerCertificates[0])
			switch was, ok := k[addr]; {
			case !ok:
				return &lanDoubt{addr: addr, key: key}
			case was != key:
				return &lanDoubt{addr: addr, key: key, was: was}
			}
			return nil
		},
	}
}

// lanFirstBytes keeps the start of what a host sends. A host refuses some people
// before encryption begins, and this is how their reason is read back.
type lanFirstBytes struct {
	net.Conn
	seen []byte
}

func (f *lanFirstBytes) Read(b []byte) (int, error) {
	n, err := f.Conn.Read(b)
	if room := lanHelloLimit - len(f.seen); room > 0 {
		f.seen = append(f.seen, b[:min(n, room)]...)
	}
	return n, err
}

// refusal is the reason a host gave in plain text instead of beginning TLS, if
// it did: one line, as an old host or a full one would send.
func (f *lanFirstBytes) refusal() (string, bool) {
	if !bytes.HasPrefix(f.seen, []byte("NO ")) {
		return "", false
	}
	_ = f.Conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 256)
	for bytes.IndexByte(f.seen, '\n') < 0 && len(f.seen) < lanHelloLimit {
		if n, err := f.Read(buf); n == 0 || err != nil {
			break
		}
	}
	line, _, _ := bytes.Cut(f.seen, []byte("\n"))
	return string(line), true
}
