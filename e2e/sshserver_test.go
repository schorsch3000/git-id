package e2e

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

// execRecord is one git command a client asked the server to run.
type execRecord struct {
	Key     string // name of the authenticated key
	Command string
	Repo    string
	Allowed bool
}

// sshServer is a minimal in-process ssh server. It authenticates by public
// key only and serves git-receive-pack / git-upload-pack for bare repos under
// root, restricting each key to one repo so that using the wrong key fails.
type sshServer struct {
	root string
	port int
	cfg  *ssh.ServerConfig

	mu      sync.Mutex
	keys    map[string]string // fingerprint -> key name
	allow   map[string]string // key name -> repo path ("/work.git")
	records []execRecord
}

func newSSHServer(t *testing.T, root string) *sshServer {
	t.Helper()
	s := &sshServer{root: root, keys: map[string]string{}, allow: map[string]string{}}
	s.cfg = &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			name, ok := s.keys[ssh.FingerprintSHA256(key)]
			if !ok {
				return nil, errors.New("unknown key")
			}
			return &ssh.Permissions{Extensions: map[string]string{"key": name}}, nil
		},
	}
	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatal(err)
	}
	s.cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go s.handleConn(nc)
		}
	}()
	return s
}

// authorize lets the key log in under name and access only repo.
func (s *sshServer) authorize(name string, pub ssh.PublicKey, repo string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[ssh.FingerprintSHA256(pub)] = name
	s.allow[name] = repo
}

func (s *sshServer) last(t *testing.T) execRecord {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) == 0 {
		t.Fatal("server saw no git command")
	}
	return s.records[len(s.records)-1]
}

func (s *sshServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

func (s *sshServer) url(repo string) string {
	return fmt.Sprintf("ssh://git@127.0.0.1:%d%s", s.port, repo)
}

func (s *sshServer) handleConn(nc net.Conn) {
	conn, chans, reqs, err := ssh.NewServerConn(nc, s.cfg)
	if err != nil {
		nc.Close()
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		if nch.ChannelType() != "session" {
			_ = nch.Reject(ssh.UnknownChannelType, "session only")
			continue
		}
		ch, creqs, err := nch.Accept()
		if err != nil {
			continue
		}
		go s.session(conn.Permissions.Extensions["key"], ch, creqs)
	}
}

func (s *sshServer) session(key string, ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	for req := range reqs {
		if req.Type != "exec" {
			_ = req.Reply(false, nil)
			continue
		}
		var p struct{ Command string }
		if ssh.Unmarshal(req.Payload, &p) != nil {
			_ = req.Reply(false, nil)
			return
		}
		_ = req.Reply(true, nil)
		status := s.exec(key, p.Command, ch)
		_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
		return
	}
}

func (s *sshServer) exec(key, command string, ch ssh.Channel) uint32 {
	name, arg, ok := strings.Cut(command, " ")
	if !ok || (name != "git-receive-pack" && name != "git-upload-pack") {
		fmt.Fprintf(ch.Stderr(), "unsupported command %q\n", command)
		return 127
	}
	repo := strings.Trim(arg, "'")
	s.mu.Lock()
	allowed := s.allow[key] == repo
	s.records = append(s.records, execRecord{Key: key, Command: name, Repo: repo, Allowed: allowed})
	s.mu.Unlock()
	if !allowed {
		fmt.Fprintf(ch.Stderr(), "key %s may not access %s\n", key, repo)
		return 1
	}

	cmd := exec.Command(name, filepath.Join(s.root, filepath.Clean("/"+repo)))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 1
	}
	cmd.Stdout = ch
	cmd.Stderr = ch.Stderr()
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(ch.Stderr(), "%v\n", err)
		return 1
	}
	// Not cmd.Stdin = ch: Wait would block until the client closes stdin.
	go func() {
		_, _ = io.Copy(stdin, ch)
		stdin.Close()
	}()
	if err := cmd.Wait(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return uint32(ee.ExitCode())
		}
		return 1
	}
	return 0
}

// writeKeypair creates an ed25519 keypair at dir/name and dir/name.pub.
func writeKeypair(t *testing.T, dir, name string) (priv, pub string, key ssh.PublicKey) {
	t.Helper()
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(privKey, "git-id e2e "+name)
	if err != nil {
		t.Fatal(err)
	}
	key, err = ssh.NewPublicKey(pubKey)
	if err != nil {
		t.Fatal(err)
	}
	priv = filepath.Join(dir, name)
	pub = priv + ".pub"
	if err := os.WriteFile(priv, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pub, ssh.MarshalAuthorizedKey(key), 0o644); err != nil {
		t.Fatal(err)
	}
	return priv, pub, key
}
