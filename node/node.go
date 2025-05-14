package node

import "time"

type NodeCfg struct {
	// Addressing
	NodeAddr  string
	Bootstrap bool
	SeedAddr  string
	// Stores
	KeyStoreDir   string
	BlockStoreDir string
	// Genesis
	ChainName string
	AuthPass  string // password for creating and initializing the authority account
	OwnerPass string
	Balance   uint64 // initial balance for the authority account
	// Processes
	Period time.Duration
}

func NewNodeCfg(nodeAddr string, bootstrap bool, seedAddr string, KeyStoreDir string, blockStoreDir string, chainName string, authPass string, ownerPass string, balance uint64) *NodeCfg {
	return &NodeCfg{
		NodeAddr:      nodeAddr,
		Bootstrap:     bootstrap,
		SeedAddr:      seedAddr,
		KeyStoreDir:   KeyStoreDir,
		BlockStoreDir: blockStoreDir,
		ChainName:     chainName,
		AuthPass:      authPass,
		OwnerPass:     ownerPass,
		Balance:       balance,
	}
}
