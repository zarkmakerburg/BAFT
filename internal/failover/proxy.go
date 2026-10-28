package failover

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Target struct {
	Name string
	Address string
	// Probe, when set, is an echo endpoint reached through the same path as
	// Address. The path counts as healthy only while a probe of ProbeBytes
	// comes back intact, which catches paths that accept connections but
	// stop carrying data after a few kilobytes.
	Probe string
}

type Proxy struct {
	Listen string
	Targets []Target
	DialTimeout time.Duration
	Health HealthConfig
	health *healthState
	failovers atomic.Uint64
	active atomic.Int64
}

func (p *Proxy) Failovers() uint64 { return p.failovers.Load() }
func (p *Proxy) Active() int64 { return p.active.Load() }

func (p *Proxy) validate() error {
	if p.Listen==""{return errors.New("failover listen address is required")}
	if len(p.Targets)<2{return errors.New("failover requires primary and at least one backup target")}
	for _,t:=range p.Targets{if t.Name==""||t.Address==""{return errors.New("failover target name/address is required")}}
	if p.DialTimeout==0{p.DialTimeout=750*time.Millisecond}
	if p.DialTimeout<10*time.Millisecond||p.DialTimeout>10*time.Second{return errors.New("invalid failover dial timeout")}
	return p.Health.normalize()
}

func (p *Proxy) Run(ctx context.Context) error {
	if err:=p.validate();err!=nil{return err}
	ln,err:=net.Listen("tcp",p.Listen);if err!=nil{return err}
	defer ln.Close()
	p.startHealth(ctx)
	go func(){<-ctx.Done();_ = ln.Close()}()
	var wg sync.WaitGroup
	defer wg.Wait()
	for{
		c,err:=ln.Accept()
		if err!=nil{
			if ctx.Err()!=nil{return nil}
			return err
		}
		wg.Add(1)
		go func(client net.Conn){defer wg.Done();p.handle(ctx,client)}(c)
	}
}

func (p *Proxy) dial(ctx context.Context) (net.Conn,int,error) {
	d:=net.Dialer{Timeout:p.DialTimeout}
	var last error
	// Healthy targets first in configured order; when none is healthy every
	// target is still tried, so a broken prober never blocks all traffic.
	for _,onlyHealthy:=range []bool{true,false}{
		for i,t:=range p.Targets{
			if p.healthy(i)!=onlyHealthy{continue}
			c,err:=d.DialContext(ctx,"tcp",t.Address)
			if err==nil{return c,i,nil}
			last=err
		}
	}
	return nil,-1,last
}

func (p *Proxy) handle(ctx context.Context,client net.Conn){
	defer client.Close()
	upstream,index,err:=p.dial(ctx);if err!=nil{return}
	defer upstream.Close()
	if index>0{p.failovers.Add(1)}
	p.active.Add(1);defer p.active.Add(-1)
	done:=make(chan struct{},2)
	go func(){_,_=io.Copy(upstream,client);if c,ok:=upstream.(*net.TCPConn);ok{_ = c.CloseWrite()};done<-struct{}{}}()
	go func(){_,_=io.Copy(client,upstream);if c,ok:=client.(*net.TCPConn);ok{_ = c.CloseWrite()};done<-struct{}{}}()
	select{
	case <-ctx.Done():
	case <-done:
	}
}
