#!/usr/bin/env python3
"""Tiny TCP helper for pair_and_run.sh.

  echo.py serve PORT              echo every connection back until EOF
  echo.py send PORT MIB CONNS     send random data over CONNS parallel
                                  connections and require identical bytes back
  echo.py soak PORT SECONDS       one connection streaming random data for
                                  SECONDS, then require the identical bytes back
"""
import hashlib, os, socket, socketserver, sys, threading, time


class Echo(socketserver.BaseRequestHandler):
    def handle(self):
        while True:
            b = self.request.recv(65536)
            if not b:
                break
            self.request.sendall(b)
        self.request.shutdown(socket.SHUT_WR)


class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True


def one(port, size, errors, idx):
    try:
        data = os.urandom(size)
        s = socket.create_connection(("127.0.0.1", port), timeout=60)
        s.settimeout(60)
        got = bytearray()

        def reader():
            while True:
                b = s.recv(65536)
                if not b:
                    return
                got.extend(b)

        t = threading.Thread(target=reader)
        t.start()
        s.sendall(data)
        s.shutdown(socket.SHUT_WR)
        t.join(120)
        s.close()
        want, have = hashlib.sha256(data).hexdigest(), hashlib.sha256(bytes(got)).hexdigest()
        if len(got) != size or want != have:
            errors.append(f"conn {idx}: sent {size} bytes sha256={want}, got {len(got)} sha256={have}")
        else:
            print(f"conn {idx}: {size} bytes round-tripped, sha256={want}")
    except Exception as e:  # noqa: BLE001 - report every failure
        errors.append(f"conn {idx}: {e!r}")


def soak(port, seconds):
    s = socket.create_connection(("127.0.0.1", port), timeout=60)
    s.settimeout(60)
    sent, got = hashlib.sha256(), hashlib.sha256()
    counts = {"sent": 0, "got": 0}

    def reader():
        while True:
            b = s.recv(65536)
            if not b:
                return
            got.update(b)
            counts["got"] += len(b)

    t = threading.Thread(target=reader)
    t.start()
    end = time.time() + seconds
    while time.time() < end:
        b = os.urandom(32768)
        s.sendall(b)
        sent.update(b)
        counts["sent"] += len(b)
        time.sleep(0.005)
    s.shutdown(socket.SHUT_WR)
    t.join(120)
    s.close()
    if counts["sent"] == 0 or counts["sent"] != counts["got"] or sent.hexdigest() != got.hexdigest():
        print(f"soak: sent {counts['sent']} got {counts['got']} sha mismatch", file=sys.stderr)
        sys.exit(1)
    print(f"soak: {counts['sent']} bytes round-tripped for {seconds}s, sha256={sent.hexdigest()}")


def main():
    if sys.argv[1] == "serve":
        Server(("127.0.0.1", int(sys.argv[2])), Echo).serve_forever()
    elif sys.argv[1] == "soak":
        soak(int(sys.argv[2]), float(sys.argv[3]))
    elif sys.argv[1] == "send":
        port, mib, conns = int(sys.argv[2]), int(sys.argv[3]), int(sys.argv[4])
        errors, threads = [], []
        for i in range(conns):
            t = threading.Thread(target=one, args=(port, mib << 20, errors, i))
            t.start()
            threads.append(t)
        for t in threads:
            t.join()
        if errors:
            print("\n".join(errors), file=sys.stderr)
            sys.exit(1)
    else:
        sys.exit("usage: echo.py serve PORT | send PORT MIB CONNS | soak PORT SECONDS")


if __name__ == "__main__":
    main()
