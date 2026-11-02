package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const lockTimeout = 30 * time.Minute

type lockContent struct {
	Token   string `json:"token"`
	PID     int    `json:"pid"`
	Host    string `json:"host,omitempty"`
	Time    string `json:"time"`
	Command string `json:"command,omitempty"`
}

// Lock is a held workspace mutation lock.
type Lock struct {
	path  string
	token string
	held  bool
}

// AcquireLock takes an exclusive advisory lock for mutating operations. It is
// not a security boundary; it prevents two Stratum processes from forking the
// same ledger. force overrides a lock that looks active (use after confirming
// no other Stratum process is running).
func (w *Workspace) AcquireLock(force bool, command string) (*Lock, error) {
	path := filepath.Join(w.Root, LockFile)
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			token := randomToken()
			host, _ := os.Hostname()
			content := lockContent{
				Token:   token,
				PID:     os.Getpid(),
				Host:    host,
				Time:    time.Now().UTC().Format(time.RFC3339Nano),
				Command: command,
			}
			data, _ := json.Marshal(content)
			if _, werr := f.Write(append(data, '\n')); werr != nil {
				_ = f.Close()
				_ = os.Remove(path)
				return nil, werr
			}
			if serr := f.Sync(); serr != nil {
				_ = f.Close()
				_ = os.Remove(path)
				return nil, serr
			}
			if cerr := f.Close(); cerr != nil {
				_ = os.Remove(path)
				return nil, cerr
			}
			return &Lock{path: path, token: token, held: true}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}

		info := readLock(path)
		stale := false
		if st, serr := os.Stat(path); serr == nil {
			stale = time.Since(st.ModTime()) > lockTimeout
		}
		host, _ := os.Hostname()
		sameHost := info.Host == "" || info.Host == host
		if sameHost && info.PID != 0 && !processAlive(info.PID) {
			stale = true
		}
		if force || stale {
			if rerr := os.Remove(path); rerr != nil && !os.IsNotExist(rerr) {
				return nil, fmt.Errorf("cannot break stale lock %s: %w", path, rerr)
			}
			continue
		}
		who := "another stratum process"
		if info.PID != 0 {
			who = fmt.Sprintf("pid %d on %s since %s", info.PID, info.Host, info.Time)
		}
		return nil, fmt.Errorf("workspace is locked by %s (%s); wait for it to finish, or use --force-lock only if you are sure it is not running", who, path)
	}
	return nil, fmt.Errorf("could not acquire workspace lock %s", path)
}

// Release removes the lock if this process still owns it.
func (l *Lock) Release() error {
	if l == nil || !l.held {
		return nil
	}
	l.held = false
	info := readLock(l.path)
	if info.Token != "" && info.Token != l.token {
		return fmt.Errorf("lock %s changed owner; not removing it", l.path)
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func readLock(path string) lockContent {
	var out lockContent
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

func randomToken() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
