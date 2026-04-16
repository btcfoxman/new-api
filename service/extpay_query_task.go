package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

func StartExtPayQueryTask() {
	go func() {
		if !common.IsMasterNode {
			return
		}
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			runExtPayQueryTaskOnce()
			<-ticker.C
		}
	}()
}

func runExtPayQueryTaskOnce() {
	locked, err := acquireExtPayQueryTaskLock()
	if err != nil {
		common.SysError("failed to acquire extpay query task lock: " + err.Error())
		return
	}
	if !locked {
		return
	}
	SyncPendingExtPayOrders(50)
}
