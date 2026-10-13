// Package geoip maps a server's IP address to its country, offline.
//
// The table is built by scripts/geoip-build.py from the public-domain
// "server-country" dataset of sapics/ip-location-db (PDDL 1.0) and embedded
// in the binary, so a lookup never sends an address anywhere.
package geoip

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/binary"
	"errors"
	"io"
	"net/netip"
	"sort"
	"sync"
)

//go:embed server-country.bin.gz
var data []byte

type table struct {
	starts []uint64
	codes  []uint8
}

var (
	once      sync.Once
	countries []string
	v4, v6    table
	loadErr   error
)

// Country returns the ISO 3166-1 alpha-2 code for ip, or false when the
// address is private, reserved or not in the table.
func Country(ip netip.Addr) (string, bool) {
	once.Do(load)
	if loadErr != nil || !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return "", false
	}
	ip = ip.Unmap()
	var key uint64
	t := &v6
	if ip.Is4() {
		b := ip.As4()
		key, t = uint64(binary.BigEndian.Uint32(b[:])), &v4
	} else {
		b := ip.As16()
		key = binary.BigEndian.Uint64(b[:8])
	}
	i := sort.Search(len(t.starts), func(i int) bool { return t.starts[i] > key }) - 1
	if i < 0 || t.codes[i] == 0 || int(t.codes[i]) > len(countries) {
		return "", false
	}
	return countries[t.codes[i]-1], true
}

func load() {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		loadErr = err
		return
	}
	r := bufio.NewReader(zr)
	magic := make([]byte, 5)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != "BGEO1" {
		loadErr = errors.New("geoip: bad table")
		return
	}
	n, err := binary.ReadUvarint(r)
	if err != nil || n > 255 {
		loadErr = errors.New("geoip: bad country list")
		return
	}
	countries = make([]string, n)
	for i := range countries {
		cc := make([]byte, 2)
		if _, err := io.ReadFull(r, cc); err != nil {
			loadErr = err
			return
		}
		countries[i] = string(cc)
	}
	for _, t := range []*table{&v4, &v6} {
		if loadErr = readTable(r, t); loadErr != nil {
			return
		}
	}
}

func readTable(r *bufio.Reader, t *table) error {
	n, err := binary.ReadUvarint(r)
	if err != nil || n > 1<<24 {
		return errors.New("geoip: bad table size")
	}
	t.starts, t.codes = make([]uint64, n), make([]uint8, n)
	var prev uint64
	for i := range t.starts {
		d, err := binary.ReadUvarint(r)
		if err != nil {
			return err
		}
		c, err := binary.ReadUvarint(r)
		if err != nil || c > 255 {
			return errors.New("geoip: bad entry")
		}
		prev += d
		t.starts[i], t.codes[i] = prev, uint8(c)
	}
	return nil
}
