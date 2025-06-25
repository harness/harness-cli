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

package xApiKey

import (
	"net/http"

	"github.com/harness/harness-cli/module/ar/migrate/lib"
)

// NewAuthorizer return a basic authorizer
func NewAuthorizer(token string) lib.Authorizer {
	return &authorizer{
		token: token,
	}
}

type authorizer struct {
	token string
}

func (a *authorizer) Modify(req *http.Request) error {
	req.Header.Add("x-api-key", a.token)
	return nil
}
