// Copyright (C) 2025-2026 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const playbackServiceUnit = "play.video.service"

var (
	dbusOperationTimeout              = 10 * time.Second
	playbackServiceOperationTimeout   = 30 * time.Second
	playbackServiceActiveCheckTimeout = 2 * time.Second
	errDBusUnitOperationTimeout       = errors.New("timeout waiting for systemd unit operation")
)

type dbusUnitOperation string

const (
	dbusUnitOperationStart   dbusUnitOperation = "start"
	dbusUnitOperationStop    dbusUnitOperation = "stop"
	dbusUnitOperationRestart dbusUnitOperation = "restart"
)

func runDBusUnitOperation(parent context.Context, conn DBusConnection, operation dbusUnitOperation, unit string) (string, error) {
	timeout := dbusOperationTimeout
	if unit == playbackServiceUnit {
		timeout = playbackServiceOperationTimeout
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	ch := make(chan string, 1)
	var err error
	switch operation {
	case dbusUnitOperationStart:
		_, err = conn.StartUnitContext(ctx, unit, "replace", ch)
	case dbusUnitOperationStop:
		_, err = conn.StopUnitContext(ctx, unit, "replace", ch)
	case dbusUnitOperationRestart:
		_, err = conn.RestartUnitContext(ctx, unit, "replace", ch)
	default:
		return "", fmt.Errorf("unknown unit operation: %s", operation)
	}
	if err != nil {
		return "", err
	}

	select {
	case result := <-ch:
		return result, nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			if playbackServiceReachedTargetState(parent, conn, operation, unit) {
				return "done", nil
			}
			return "", errDBusUnitOperationTimeout
		}
		return "", ctx.Err()
	}
}

func playbackServiceReachedTargetState(parent context.Context, conn DBusConnection, operation dbusUnitOperation, unit string) bool {
	if unit != playbackServiceUnit {
		return false
	}

	statusCtx, statusCancel := context.WithTimeout(parent, playbackServiceActiveCheckTimeout)
	defer statusCancel()

	switch operation {
	case dbusUnitOperationStart, dbusUnitOperationRestart:
		return isUnitActive(statusCtx, conn, unit)
	case dbusUnitOperationStop:
		return isUnitNotActive(statusCtx, conn, unit)
	default:
		return false
	}
}

func unitActiveState(ctx context.Context, conn DBusConnection, unit string) (string, bool) {
	props, err := conn.GetUnitPropertiesContext(ctx, unit)
	if err != nil {
		return "", false
	}
	state, ok := props["ActiveState"].(string)
	return state, ok
}

func isUnitActive(ctx context.Context, conn DBusConnection, unit string) bool {
	state, ok := unitActiveState(ctx, conn, unit)
	return ok && state == "active"
}

func isUnitNotActive(ctx context.Context, conn DBusConnection, unit string) bool {
	state, ok := unitActiveState(ctx, conn, unit)
	return ok && state != "active"
}
