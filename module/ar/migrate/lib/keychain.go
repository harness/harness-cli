package lib

import (
	"github.com/harness/harness-cli/module/ar/migrate/adapter"

	"github.com/google/go-containerregistry/pkg/authn"
)

func CreateCraneKeychain(
	srcAdapter adapter.Adapter,
	destAdapter adapter.Adapter,
	srcRegistry string,
	destRegistry string,
) authn.Keychain {
	srcKeychain := srcAdapter.GetKeyChain(srcRegistry)
	dstKeychain := destAdapter.GetKeyChain(destRegistry)

	customKeychain := authn.NewMultiKeychain(
		srcKeychain,
		dstKeychain,
	)

	return customKeychain
}
