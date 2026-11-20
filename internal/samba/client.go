// SPDX-License-Identifier: MIT

package samba

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// ErrUnreachable is a helper that does not answer.
var ErrUnreachable = errors.New("the Samba companion does not answer")

// Client talks to the helper of one companion over its Unix socket.
type Client struct {
	Socket  string
	Timeout time.Duration
}

// Do sends one request and reads its answer. An answer that is not OK is an error.
func (c Client) Do(ctx context.Context, r Request) (Response, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if err := json.NewEncoder(conn).Encode(r); err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if uc, ok := conn.(*net.UnixConn); ok {
		_ = uc.CloseWrite()
	}
	var answer Response
	if err := json.NewDecoder(io.LimitReader(conn, 64<<20)).Decode(&answer); err != nil {
		return Response{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if !answer.OK {
		return answer, fmt.Errorf("samba companion: %s", answer.Error)
	}
	return answer, nil
}
