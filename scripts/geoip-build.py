#!/usr/bin/env python3
"""Build internal/geoip/server-country.bin.gz from sapics/ip-location-db.

Source: the "server-country" IPv4/IPv6 "num" CSVs (PDDL 1.0, public domain):
  https://github.com/sapics/ip-location-db/releases/download/latest/server-country-ipv4-num.csv
  https://github.com/sapics/ip-location-db/releases/download/latest/server-country-ipv6-num.csv

Usage: scripts/geoip-build.py IPV4_CSV IPV6_CSV [OUT]

Format (all integers unsigned LEB128 varints unless noted):
  "BGEO1"  magic
  n        number of country codes, then n x 2 ASCII bytes
  v4count  then v4count x (delta start, code index); index 0 = no country
  v6count  then v6count x (delta start, code index); IPv6 keys are the top
           64 bits of the address
Each entry covers addresses from its start up to the next entry's start.
"""
import csv, gzip, io, sys

def varint(n, out):
    while True:
        b = n & 0x7F
        n >>= 7
        if n:
            out.append(b | 0x80)
        else:
            out.append(b)
            return

def ranges(path, shift):
    rows = []
    for a, b, cc in csv.reader(open(path)):
        rows.append((int(a) >> shift, int(b) >> shift, cc.strip().upper()))
    rows.sort()
    out = []  # (start, cc or "")
    end = -1
    for a, b, cc in rows:
        if a <= end:  # overlap after truncation: keep the earlier owner
            a = end + 1
            if a > b:
                continue
        if a > end + 1:
            out.append((end + 1, ""))
        if out and out[-1][1] == cc:
            pass
        else:
            out.append((a, cc))
        end = b
    out.append((end + 1, ""))
    # drop a leading gap at 0 and merge repeats
    merged = []
    for s, cc in out:
        if merged and merged[-1][1] == cc:
            continue
        merged.append((s, cc))
    return merged

def main():
    v4 = ranges(sys.argv[1], 0)
    v6 = ranges(sys.argv[2], 64)
    codes = sorted({cc for _, cc in v4 + v6 if cc})
    index = {cc: i + 1 for i, cc in enumerate(codes)}
    buf = bytearray(b"BGEO1")
    varint(len(codes), buf)
    for cc in codes:
        buf += cc.encode("ascii")[:2].ljust(2, b"?")
    for table in (v4, v6):
        varint(len(table), buf)
        prev = 0
        for s, cc in table:
            varint(s - prev, buf)
            varint(index.get(cc, 0), buf)
            prev = s
    out = sys.argv[3] if len(sys.argv) > 3 else "internal/geoip/server-country.bin.gz"
    with open(out, "wb") as f:
        f.write(gzip.compress(bytes(buf), 9, mtime=0))
    print(f"{out}: {len(codes)} countries, {len(v4)} IPv4 and {len(v6)} IPv6 ranges, {len(buf)} bytes raw")

main()
