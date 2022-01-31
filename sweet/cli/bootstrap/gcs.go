// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"io"

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

func UploadArchive(r io.Reader, bucket, version string, auth AuthOption, force, public bool) error {
	ctx := context.Background()
	var opts []option.ClientOption
	switch auth {
	case AuthAppDefault:
		creds, err := google.FindDefaultCredentials(ctx, storage.ScopeReadWrite)
		if err != nil {
			return err
		}
		opts = append(opts, option.WithCredentials(creds))
	case AuthNone:
		return fmt.Errorf("authentication required for upload")
	default:
		return fmt.Errorf("unknown authentication method")
	}
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return err
	}
	o := client.Bucket(bucket).Object(VersionArchiveName(version))
	if _, err := o.Attrs(ctx); err != nil && err != storage.ErrObjectNotExist {
		return fmt.Errorf("checking if object exists: %v", err)
	} else if err == nil && !force {
		return fmt.Errorf("assets object already exists for version %s", version)
	}

	// Write the archive to GCS.
	wc := o.NewWriter(ctx)
	if _, err = io.Copy(wc, r); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}

	if public {
		// Make the archive public.
		acl := o.ACL()
		return acl.Set(ctx, storage.AllUsers, storage.RoleReader)
	}
	return nil
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
