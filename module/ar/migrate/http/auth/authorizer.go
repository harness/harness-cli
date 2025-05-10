// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"net/http"
	"net/url"
	"strings"
	"sync"

	commonhttp "harness/module/ar/migrate/http"
	"harness/module/ar/migrate/http/auth/bearer"
	"harness/module/ar/migrate/http/modifier"
	"harness/module/ar/migrate/lib"
)

// NewAuthorizer creates an authorizer that can handle different auth schemes
func NewAuthorizer(username, password string, insecure bool) lib.Authorizer {
	return &authorizer{
		username: username,
		password: password,
		client: &http.Client{
			Transport: commonhttp.GetHTTPTransport(commonhttp.WithInsecure(insecure)),
		},
	}
}

// authorizer authorizes the request with the provided credential.
// It determines the auth scheme of registry automatically and calls
// different underlying authorizers to do the auth work
type authorizer struct {
	sync.Mutex
	username   string
	password   string
	client     *http.Client
	url        *url.URL          // registry URL
	authorizer modifier.Modifier // the underlying authorizer
}

func (a *authorizer) Modify(req *http.Request) error {
	// Nil underlying authorizer means this is the first time the authorizer is called
	// Try to connect to the registry and determine the auth scheme
	if a.authorizer == nil {
		// to avoid concurrent issue
		a.Lock()
		defer a.Unlock()
		if err := a.initialize(req.URL); err != nil {
			return err
		}
	}

	// check whether the request targets the registry
	if !a.isTarget(req) {
		return nil
	}

	return a.authorizer.Modify(req)
}

func (a *authorizer) initialize(u *url.URL) error {
	a.authorizer = bearer.NewAuthorizer(a.password)
	return nil
}

// Check whether the request targets to the registry.
// If doesn't, the request shouldn't be handled by the authorizer.
// e.g. the requests sent to backend storage(s3, etc.)
func (a *authorizer) isTarget(req *http.Request) bool {
	index := strings.Index(req.URL.Path, "/v2/")
	if index == -1 {
		return false
	}
	if req.URL.Host != a.url.Host || req.URL.Scheme != a.url.Scheme ||
		req.URL.Path[:index+4] != a.url.Path {
		return false
	}
	return true
}
