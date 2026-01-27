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

package bearer

import (
	"fmt"
	"net/http"

	"github.com/harness/harness-cli/module/ar/migrate/lib"
)

const (
	cacheCapacity = 100
)

// NewAuthorizer return a bearer token authorizer
// The parameter "a" is an authorizer used to fetch the token
func NewAuthorizer(token string) lib.Authorizer {
	authorizer := &authorizer{
		//realm:      realm,
		//service:    service,
		//authorizer: a,
		//cache:      newCache(cacheCapacity),
	}

	//authorizer.client = &http.Client{Transport: transport}
	authorizer.token = token
	return authorizer
}

type authorizer struct {
	realm      string
	service    string
	authorizer lib.Authorizer
	client     *http.Client
	token      string
}

func (a *authorizer) Modify(req *http.Request) error {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", a.token))
	return nil
}
