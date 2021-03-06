// Copyright 2014, Google Inc. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etcd3topo

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/tiglabs/baudengine/proto/metapb"
	"github.com/tiglabs/baudengine/topo"
)

// cellClient wraps a Client for keeping track of cell-local clusters.
type cellClient struct {
	// cli is the v3 client.
	cli *clientv3.Client

	// root is the root path for this client.
	root string
}

// newCellClient returns a new cellClient for the given address and root.
func newCellClient(serverAddr, root string) (*cellClient, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   strings.Split(serverAddr, ","),
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	return &cellClient{
		cli:  cli,
		root: root,
	}, nil
}

func (c *cellClient) close() {
	c.cli.Close()
}

// cell returns a client for the given cell-local etcd cluster.
// It caches clients for previously requested cells.
func (s *Server) clientForCell(ctx context.Context, cell string) (*cellClient, error) {
	// Global cell is the easy case.
	if cell == topo.GlobalZone {
		return s.global, nil
	}

	// Return a cached client if present.
	s.mu.Lock()
	client, ok := s.cells[cell]
	s.mu.Unlock()
	if ok {
		return client, nil
	}

	// Fetch cell cluster addresses from the global cluster.
	// These can proceed concurrently (we've released the lock).
	serverAddr, root, err := s.getCellAddrs(ctx, cell)
	if err != nil {
		return nil, err
	}

	// Update the cache.
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if another goroutine beat us to creating a client for
	// this cell.
	if client, ok = s.cells[cell]; ok {
		return client, nil
	}

	// Create the client.
	c, err := newCellClient(serverAddr, root)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %v: %v", serverAddr, err)
	}
	s.cells[cell] = c
	return c, nil
}

// getCellAddrs returns the list of etcd servers to try for the given
// cell-local cluster, and the root directory. These lists are stored
// in the global etcd cluster.
func (s *Server) getCellAddrs(ctx context.Context, cell string) (string, string, error) {
	nodePath := path.Join(s.global.root, topo.ZonesPath, cell, topo.ZoneTopoFile)
	resp, err := s.global.cli.Get(ctx, nodePath)
	if err != nil {
		return "", "", convertError(err)
	}
	if len(resp.Kvs) != 1 {
		return "", "", topo.ErrNoNode
	}
	ci := &metapb.Zone{}
	if err := proto.Unmarshal(resp.Kvs[0].Value, ci); err != nil {
		return "", "", fmt.Errorf("cannot unmarshal cell node %v: %v", nodePath, err)
	}
	if ci.ServerAddrs == "" {
		return "", "", fmt.Errorf("CellInfo.ServerAddress node %v is empty, expected list of addresses", nodePath)
	}

	return ci.ServerAddrs, ci.RootDir, nil
}
