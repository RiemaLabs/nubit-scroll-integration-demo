package core

import (
	"context"
	"encoding/hex"
	"github.com/rollkit/go-da/proxy"
	"testing"
)

func TestNubit(t *testing.T) {
	txs := []byte("test txs")
	cn, err := proxy.NewClient("http://127.0.0.1:26658", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJBbGxvdyI6WyJwdWJsaWMiLCJyZWFkIiwid3JpdGUiLCJhZG1pbiJdfQ.DAMv0s7915Ahx-kDFSzDT1ATz4Q9WwktWcHmjp7_99Q")
	if err != nil {
		t.Fatal(err)
	}
	namespaceStr := "0000000000000000000000000000000000000000000000007363726f6c6c"
	namespaceHex, err := hex.DecodeString(namespaceStr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cn.Submit(context.TODO(), [][]byte{txs}, -1, namespaceHex)
	if err != nil {
		t.Fatal(err)
	}
}
