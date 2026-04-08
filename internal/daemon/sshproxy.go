package daemon

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

const proxyPortBase = 7100

// sshProxy manages per-smurf TCP listeners that forward to the smurf's SSH.
type sshProxy struct {
	mu        sync.Mutex
	listeners map[string]*proxyEntry // smurfID -> entry
	nextPort  int
}

type proxyEntry struct {
	smurfID  string
	port     int
	targetIP string
	listener net.Listener
	done     chan struct{}
}

func newSSHProxy() *sshProxy {
	return &sshProxy{
		listeners: make(map[string]*proxyEntry),
		nextPort:  proxyPortBase,
	}
}

// Start opens a TCP listener that forwards connections to targetIP:22.
// Returns the allocated port.
func (p *sshProxy) Start(smurfID, targetIP string) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Already running?
	if e, ok := p.listeners[smurfID]; ok {
		return e.port, nil
	}

	port := p.nextPort
	p.nextPort++

	addr := fmt.Sprintf("0.0.0.0:%d", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("listen %s: %w", addr, err)
	}

	entry := &proxyEntry{
		smurfID:  smurfID,
		port:     port,
		targetIP: targetIP,
		listener: lis,
		done:     make(chan struct{}),
	}
	p.listeners[smurfID] = entry

	go entry.serve()
	slog.Info("ssh proxy started", "smurf", smurfID, "port", port, "target", targetIP)
	return port, nil
}

// Stop closes the proxy listener for a smurf.
func (p *sshProxy) Stop(smurfID string) {
	p.mu.Lock()
	entry, ok := p.listeners[smurfID]
	if ok {
		delete(p.listeners, smurfID)
	}
	p.mu.Unlock()

	if ok {
		entry.listener.Close()
		<-entry.done
		slog.Info("ssh proxy stopped", "smurf", smurfID, "port", entry.port)
	}
}

// Port returns the proxy port for a smurf, or 0 if not running.
func (p *sshProxy) Port(smurfID string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e, ok := p.listeners[smurfID]; ok {
		return e.port
	}
	return 0
}

// StopAll closes all proxy listeners.
func (p *sshProxy) StopAll() {
	p.mu.Lock()
	entries := make([]*proxyEntry, 0, len(p.listeners))
	for _, e := range p.listeners {
		entries = append(entries, e)
	}
	p.listeners = make(map[string]*proxyEntry)
	p.mu.Unlock()

	for _, e := range entries {
		e.listener.Close()
		<-e.done
	}
}

func (e *proxyEntry) serve() {
	defer close(e.done)
	for {
		conn, err := e.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go e.forward(conn)
	}
}

func (e *proxyEntry) forward(client net.Conn) {
	defer client.Close()

	// Enable TCP keepalive on the client side to prevent idle timeouts
	if tc, ok := client.(*net.TCPConn); ok {
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(15 * time.Second)
	}

	target, err := net.DialTimeout("tcp", net.JoinHostPort(e.targetIP, "22"), 5*time.Second)
	if err != nil {
		slog.Debug("ssh proxy dial failed", "smurf", e.smurfID, "err", err)
		return
	}
	defer target.Close()

	// Enable TCP keepalive on the target side too
	if tc, ok := target.(*net.TCPConn); ok {
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(15 * time.Second)
	}

	done := make(chan struct{}, 2)
	go func() { io.Copy(target, client); done <- struct{}{} }()
	go func() { io.Copy(client, target); done <- struct{}{} }()
	<-done
}
