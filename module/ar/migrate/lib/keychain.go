package lib

import (
	"github.com/harness/harness-cli/module/ar/migrate/adapter"

	"github.com/google/go-containerregistry/pkg/authn"
)

func CreateCraneKeychain(
	srcAdapter adapter.Adapter,
	destAdapter adapter.Adapter,
	sourcePackageHostname string,
) (authn.Keychain, error) {
	srcKeychain, err := srcAdapter.GetKeyChain(sourcePackageHostname)
	if err != nil {
		return nil, err
	}
	dstKeychain, err := destAdapter.GetKeyChain("")
	if err != nil {
		return nil, err
	}

	customKeychain := authn.NewMultiKeychain(
		srcKeychain,
		dstKeychain,
	)

	return customKeychain, nil
}
