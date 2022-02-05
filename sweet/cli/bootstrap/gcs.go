// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"

	"golang.org/x/oauth2/google"

	"google.golang.org/api/option"
)

type AuthOption int

const (
	AuthNone AuthOption = iota
	AuthAppDefault
	NumAuthOptions
)

var authOptString = [NumAuthOptions]string{
	"none",
	"app-default",
}

func (a *AuthOption) String() string {
	return authOptString[*a]
}

func (a *AuthOption) Set(input string) error {
	for i := range authOptString {
		if authOptString[i] == input {
			*a = AuthOption(i)
			return nil
		}
	}
	return fmt.Errorf("unrecognized authentication option: %s", input)
}

func NewStorageWriter(bucket, version string, auth AuthOption, force bool) (*storage.Writer, error) {
	ctx := context.Background()
	var opts []option.ClientOption
	switch auth {
	case AuthAppDefault:
		creds, err := google.FindDefaultCredentials(ctx, storage.ScopeReadWrite)
		if err != nil {
			return nil, err
		}
		opts = append(opts, option.WithCredentials(creds))
	case AuthNone:
		return nil, fmt.Errorf("authentication required for upload")
	default:
		return nil, fmt.Errorf("unknown authentication method")
	}
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	o := client.Bucket(bucket).Object(VersionArchiveName(version))
	if _, err := o.Attrs(ctx); err != nil && err != storage.ErrObjectNotExist {
		return nil, fmt.Errorf("checking if object exists: %w", err)
	} else if err == nil && !force {
		return nil, fmt.Errorf("assets object already exists for version %s", version)
	}
	return o.NewWriter(ctx), nil
}

func NewStorageReader(bucket, version string, auth AuthOption) (*storage.Reader, error) {
	ctx := context.Background()
	opts := []option.ClientOption{option.WithScopes(storage.ScopeReadOnly)}
	switch auth {
	case AuthAppDefault:
		creds, err := google.FindDefaultCredentials(ctx, storage.ScopeReadOnly)
		if err != nil {
			return nil, err
		}
		opts = append(opts, option.WithCredentials(creds))
	case AuthNone:
		opts = append(opts, option.WithoutAuthentication())
	default:
		return nil, fmt.Errorf("unknown authentication method")
	}
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return client.Bucket(bucket).Object(VersionArchiveName(version)).NewReader(ctx)
}
