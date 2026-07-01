package agent

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrNoAgent indicates no agent is reachable at the socket path.
var ErrNoAgent = errors.New("no pass-cli agent is running")

// sendControl dials the agent and sends a single control request (status, lock,
// or shutdown), returning the decoded response.
func sendControl(method string) (Response, error) {
	path := SocketPath()
	if path == "" {
		return Response{}, ErrNoAgent
	}
	conn, err := dialSocket(path)
	if err != nil {
		return Response{}, ErrNoAgent
	}
	defer func() { _ = conn.Close() }()

	if err := json.NewEncoder(conn).Encode(Request{Version: ProtocolVersion, Method: method}); err != nil {
		return Response{}, fmt.Errorf("failed to send %s request: %w", method, err)
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, fmt.Errorf("failed to read agent response: %w", err)
	}
	return resp, nil
}

// Stop asks a running agent to lock and shut down.
func Stop() error {
	resp, err := sendControl(MethodShutdown)
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	return nil
}

// QueryStatus returns a running agent's status snapshot.
func QueryStatus() (*Status, error) {
	resp, err := sendControl(MethodStatus)
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, errors.New(resp.Error)
	}
	return resp.Status, nil
}
