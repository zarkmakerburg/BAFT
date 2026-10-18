package bcc

import (
 "testing"
 "time"
)

func TestCheckComponentReceiptFreshness(t *testing.T) {
 now := time.Now().UTC()
 if err := CheckComponentReceiptFreshness(now, now.Add(-time.Second), now, time.Minute); err != nil { t.Fatal(err) }
 if err := CheckComponentReceiptFreshness(now, now, now, time.Minute); err == nil { t.Fatal("replay accepted") }
 if err := CheckComponentReceiptFreshness(now.Add(-time.Second), now, now, time.Minute); err == nil { t.Fatal("out-of-order accepted") }
 if err := CheckComponentReceiptFreshness(now.Add(3*time.Minute), time.Time{}, now, time.Minute); err == nil { t.Fatal("future accepted") }
 if err := CheckComponentReceiptFreshness(now.Add(-2*time.Minute), time.Time{}, now, time.Minute); err == nil { t.Fatal("stale accepted") }
}
