package integration_test

import (
		"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"sync"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/node"
	"github.com/zarkmakerburg/baft/internal/recordshape"
	"github.com/zarkmakerburg/baft/internal/securityinternal"
)

func TestNoiseListenerAcceptsAndSeparatesTwoIRPeers(t *testing.T) {
	certs:=testPKI(t)
	dir:=t.TempDir()
	write:=func(name string,b []byte) string{
		p:=filepath.Join(dir,name)
		if err:=os.WriteFile(p,b,0600);err!=nil{t.Fatal(err)}
		return p
	}
	ca:=write("ca.pem",pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:certs.caDER}))
	cert:=write("server.pem",pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:certs.server.Certificate[0]}))
	der,err:=x509.MarshalPKCS8PrivateKey(certs.server.PrivateKey);if err!=nil{t.Fatal(err)}
	tlsKey:=write("server.key",pem.EncodeToMemory(&pem.Block{Type:"PRIVATE KEY",Bytes:der}))

	newTarget := func(marker byte) net.Listener {
		t.Helper()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				go func(c net.Conn) {
					defer c.Close()
					var request [1]byte
					if _, err := io.ReadFull(c, request[:]); err == nil {
						_, _ = c.Write([]byte{marker})
					}
				}(conn)
			}
		}()
		return ln
	}
	ownerTarget := newTarget('A')
	defer ownerTarget.Close()
	tenantTarget := newTarget('B')
	defer tenantTarget.Close()

	exKey,err:=securityinternal.GenerateKeyPair();if err!=nil{t.Fatal(err)}
	ir1Key,err:=securityinternal.GenerateKeyPair();if err!=nil{t.Fatal(err)}
	ir2Key,err:=securityinternal.GenerateKeyPair();if err!=nil{t.Fatal(err)}
	exPath:=filepath.Join(dir,"ex-noise.json");ir1Path:=filepath.Join(dir,"ir1-noise.json");ir2Path:=filepath.Join(dir,"ir2-noise.json")
	for _,v:=range []struct{p string;k securityinternal.KeyPair}{{exPath,exKey},{ir1Path,ir1Key},{ir2Path,ir2Key}}{
		if err:=securityinternal.SaveKeyPair(v.p,v.k);err!=nil{t.Fatal(err)}
	}
	exPub,_:=securityinternal.EncodePublicKey(exKey.Public)
	ir1Pub,_:=securityinternal.EncodePublicKey(ir1Key.Public)
	ir2Pub,_:=securityinternal.EncodePublicKey(ir2Key.Public)

	ex,err:=config.LoadFile("../../configs/example-ex.yaml");if err!=nil{t.Fatal(err)}
	ex.Node.ID="ex-shared"
	ex.Server.Listen=reserveAddress(t)
	ex.Server.ServerName="ex.test"
	ex.Server.AllowedPeerIdentities=[]string{"urn:baft:node:ir-master"}
	ex.Management.UnixSocket=filepath.Join(dir,"ex.sock")
	ex.Management.MetricsListen=reserveAddress(t)
	ex.Transport.Shards=1
	ex.Routes[0].Target=ownerTarget.Addr().String()
	ex.Routes[0].AllowedPeers=[]string{"urn:baft:node:ir-master"}
	ex.TLS=config.TLS{MinVersion:"1.3",CAFile:ca,CertFile:cert,KeyFile:tlsKey,SessionTickets:false}
	// Start from the legacy single-peer pinned listener and use the same
	// transformation the BCC tenant lifecycle invokes to add the second peer.
	ex.Noise=&config.Noise{KeyFile:exPath,PeerPublicKey:ir1Pub,RecordShaping:recordshape.Config{}}
	ex,err=config.AddListenerTenant(ex,ex.Node.ID,ex.Server.Listen,"urn:baft:node:ir-worker",ir2Pub,"service-worker",tenantTarget.Addr().String())
	if err!=nil{t.Fatalf("add second authenticated tenant: %v",err)}

	makeIR:=func(id,keyPath,listen,metrics string) config.Config{
		ir,e:=config.LoadFile("../../configs/example-ir.yaml");if e!=nil{t.Fatal(e)}
		ir.Node.ID=id
		ir.Peer.Address=ex.Server.Listen
		ir.Peer.ServerName="ex.test"
		ir.Peer.AllowedIdentity="urn:baft:node:ex-shared"
		ir.Management.UnixSocket=filepath.Join(dir,id+".sock")
		ir.Management.MetricsListen=metrics
		ir.Transport.Shards=1
		ir.Routes[0].Listen=listen
		ir.TLS=ex.TLS
		ir.Noise=&config.Noise{KeyFile:keyPath,PeerPublicKey:exPub,RecordShaping:recordshape.Config{}}
		return ir
	}
	ir1:=makeIR("ir-master",ir1Path,reserveAddress(t),reserveAddress(t))
	ir2:=makeIR("ir-worker",ir2Path,reserveAddress(t),reserveAddress(t))
	if err:=config.Validate(ex);err!=nil{t.Fatalf("EX config: %v",err)}
	if err:=config.Validate(ir1);err!=nil{t.Fatalf("IR1 config: %v",err)}
	if err:=config.Validate(ir2);err!=nil{t.Fatalf("IR2 config: %v",err)}

	ctx,cancel:=context.WithTimeout(context.Background(),25*time.Second)
	defer cancel()
	exDone:=make(chan error,1);go func(){exDone<-node.NewRuntime().Run(ctx,ex)}()
	waitTCP(t,ex.Server.Listen,time.Now().Add(8*time.Second))

	ir1Done:=make(chan error,1);ir2Done:=make(chan error,1)
	go func(){ir1Done<-node.NewRuntime().Run(ctx,ir1)}()
	go func(){ir2Done<-node.NewRuntime().Run(ctx,ir2)}()
	waitTCP(t,ir1.Routes[0].Listen,time.Now().Add(8*time.Second))
	waitTCP(t,ir2.Routes[0].Listen,time.Now().Add(8*time.Second))

	roundtrip := func(label, addr string, want byte) {
		t.Helper()
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := c.Write([]byte{'x'}); err != nil {
			t.Fatalf("%s write: %v", label, err)
		}
		got := []byte{0}
		if _, err := io.ReadFull(c, got); err != nil {
			t.Fatalf("%s read: %v", label, err)
		}
		if got[0] != want {
			t.Fatalf("%s reached wrong tenant target: got %q want %q", label, got[0], want)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, tc := range []struct{ label, addr string; want byte }{
		{ir1.Node.ID, ir1.Routes[0].Listen, 'A'},
		{ir2.Node.ID, ir2.Routes[0].Listen, 'B'},
	} {
		wg.Add(1)
		go func(tc struct{ label, addr string; want byte }) {
			defer wg.Done()
			roundtrip(tc.label, tc.addr, tc.want)
		}(tc)
	}
	wg.Wait()
	close(errs)
	t.Logf("PASS concurrent authenticated peers were isolated to distinct targets: master=%s→A worker=%s→B", ir1.Node.ID, ir2.Node.ID)

	cancel()
	for name,ch:=range map[string]<-chan error{"ex":exDone,"ir1":ir1Done,"ir2":ir2Done}{
		select{
		case e:=<-ch:
			if e!=nil{t.Fatalf("%s runtime: %v",name,e)}
		case <-time.After(8*time.Second):
			t.Fatalf("%s shutdown timed out",name)
		}
	}
}
