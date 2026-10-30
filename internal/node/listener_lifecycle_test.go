package node

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
)

func TestListenerLifecycleClosesAdmissionBeforeJoiningHandlers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cleanup := make(chan struct{})
		calls := 0
		life := newListenerLifecycle(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			<-r.Context().Done()
			<-cleanup
		}))
		go life.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil).WithContext(life.ctx))
		synctest.Wait()
		closed := make(chan struct{})
		go func() { life.close(&http.Server{}); close(closed) }()
		synctest.Wait()
		select {
		case <-closed:
			t.Fatal("shutdown returned before handler cleanup")
		default:
		}
		late := httptest.NewRecorder()
		life.ServeHTTP(late, httptest.NewRequest("GET", "/", nil))
		if late.Code != http.StatusServiceUnavailable || calls != 1 {
			t.Fatalf("shutdown admitted a late handler: code=%d calls=%d", late.Code, calls)
		}
		close(cleanup)
		synctest.Wait()
		select {
		case <-closed:
		default:
			t.Fatal("shutdown did not join completed handler")
		}
	})
}
