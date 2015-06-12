// Copyright 2015 FactomProject Authors. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package process

import (
	"github.com/FactomProject/FactomCode/common"
	"github.com/FactomProject/btcd/wire"
	"github.com/FactomProject/FactomCode/util"	
	"github.com/davecgh/go-spew/spew"	
	"strconv"
	"sort"	
	"errors"
	"fmt"	
	"github.com/FactomProject/FactomCode/consensus"

	sc "github.com/FactomProject/factoid"
	"github.com/FactomProject/factoid/block"
)

// Initialize the entry chains in memory from db
func initEChainFromDB(chain *common.EChain) {

	eBlocks, _ := db.FetchAllEBlocksByChain(chain.ChainID)
	sort.Sort(util.ByEBlockIDAccending(*eBlocks))

	for i := 0; i < len(*eBlocks); i = i + 1 {
		if uint32(i) != (*eBlocks)[i].Header.EBHeight {
			panic(errors.New("BlockID does not equal index for chain:" + chain.ChainID.String() + " block:" + fmt.Sprintf("%v", (*eBlocks)[i].Header.EBHeight)))
		}
	}

	if len(*eBlocks) == 0 {
		chain.NextBlockHeight = 0
		chain.NextBlock, _ = common.CreateBlock(chain, nil, 10)
	} else {
		chain.NextBlockHeight = uint32(len(*eBlocks))
		chain.NextBlock, _ = common.CreateBlock(chain, &(*eBlocks)[len(*eBlocks)-1], 10)
	}

	// Initialize chain with the first entry (Name and rules) for non-server mode
	if nodeMode != common.SERVER_NODE && chain.FirstEntry == nil && len(*eBlocks) > 0 {
		chain.FirstEntry, _ = db.FetchEntryByHash((*eBlocks)[0].EBEntries[0].EntryHash)
		if chain.FirstEntry != nil {
			db.InsertChain(chain)
		}
	}

	if chain.NextBlock.IsSealed == true {
		panic("chain.NextBlock.IsSealed for chain:" + chain.ChainID.String())
	}
}

// Validate dir chain from genesis block
func validateDChain(c *common.DChain) error {

	if uint32(len(c.Blocks)) != c.NextBlockHeight {
		return errors.New("Dir chain doesn't have an expected Next Block ID: " + strconv.Itoa(int(c.NextBlockHeight)))
	}

	//prevBlk := c.Blocks[0]
	prevMR, prevBlkHash, err := validateDBlock(c, c.Blocks[0])
	if err != nil {
		return err
	}

	//validate the genesis block
	if prevBlkHash == nil || prevBlkHash.String() != common.GENESIS_DIR_BLOCK_HASH {
		panic("Genesis dir block is not as expected: " + prevBlkHash.String())
	}

	for i := 1; i < len(c.Blocks); i++ {
		if !prevBlkHash.IsSameAs(c.Blocks[i].Header.PrevBlockHash) {
			return errors.New("Previous block hash not matching for Dir block: " + strconv.Itoa(i))
		}
		if !prevMR.IsSameAs(c.Blocks[i].Header.PrevKeyMR) {
			return errors.New("Previous merkle root not matching for Dir block: " + strconv.Itoa(i))
		}
		mr, dblkHash, err := validateDBlock(c, c.Blocks[i])
		if err != nil {
			c.Blocks[i].IsValidated = false
			return err
		}

		prevMR = mr
		prevBlkHash = dblkHash
		c.Blocks[i].IsValidated = true
	}

	return nil
}

// Validate a dir block
func validateDBlock(c *common.DChain, b *common.DirectoryBlock) (merkleRoot *common.Hash, dbHash *common.Hash, err error) {

	bodyMR, err := b.BuildBodyMR()
	if err != nil {
		return nil, nil, err
	}

	if !b.Header.BodyMR.IsSameAs(bodyMR) {
		fmt.Printf("\n\nERROR!!!!!! !b.Header.BodyMR.IsSameAs(bodyMR) fails.\n\n")
		//		return nil, nil, errors.New("Invalid body MR for dir block: " + string(b.Header.BlockHeight))
	}

	for _, dbEntry := range b.DBEntries {
		switch dbEntry.ChainID.String() {
		case ecchain.ChainID.String():
			err := validateCBlockByMR(dbEntry.MerkleRoot)
			if err != nil {
				return nil, nil, err
			}
		case achain.ChainID.String():
			err := validateABlockByMR(dbEntry.MerkleRoot)
			if err != nil {
				return nil, nil, err
			}
		case scchain.ChainID.String():
			err := validateFBlockByMR(dbEntry.MerkleRoot)
			if err != nil {
				return nil, nil, err
			}
		case wire.FChainID.String():
			err := validateFBlockByMR(dbEntry.MerkleRoot)
			if err != nil {
				return nil, nil, err
			}
		default:
			err := validateEBlockByMR(dbEntry.ChainID, dbEntry.MerkleRoot)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	b.DBHash, _ = common.CreateHash(b)
	b.BuildKeyMerkleRoot()

	return b.KeyMR, b.DBHash, nil
}

func validateCBlockByMR(mr *common.Hash) error {
	cb, _ := db.FetchECBlockByHash(mr)

	if cb == nil {
		return errors.New("Entry Credit block not found in db for merkle root: " + mr.String())
	}

	return nil
}

// Validate Admin Block by merkle root
func validateABlockByMR(mr *common.Hash) error {
	b, _ := db.FetchABlockByHash(mr)

	if b == nil {
		return errors.New("Admin block not found in db for merkle root: " + mr.String())
	}

	return nil
}

// Validate FBlock by merkle root
func validateFBlockByMR(mr *common.Hash) error {
	b, _ := db.FetchFBlockByHash(mr)

	if b == nil {
		return errors.New("Simple Coin block not found in db for merkle root: " + mr.String())
	}

	return nil
}

func validateEBlockByMR(cid *common.Hash, mr *common.Hash) error {

	eb, _ := db.FetchEBlockByMR(mr)

	if eb == nil {
		return errors.New("Entry block not found in db for merkle root: " + mr.String())
	}

	eb.BuildMerkleRoot()

	if !mr.IsSameAs(eb.MerkleRoot) {
		return errors.New("Entry block's merkle root does not match with: " + mr.String())
	}

	for _, ebEntry := range eb.EBEntries {
		entry, _ := db.FetchEntryByHash(ebEntry.EntryHash)
		if entry == nil {
			return errors.New("Entry not found in db for entry hash: " + ebEntry.EntryHash.String())
		}
	}

	return nil
}


func initDChain() {
	dchain = new(common.DChain)

	//Initialize the Directory Block Chain ID
	dchain.ChainID = new(common.Hash)
	barray := common.D_CHAINID
	dchain.ChainID.SetBytes(barray)

	// get all dBlocks from db
	dBlocks, _ := db.FetchAllDBlocks()
	sort.Sort(util.ByDBlockIDAccending(dBlocks))

	//fmt.Printf("initDChain: dBlocks=%s\n", spew.Sdump(dBlocks))

	dchain.Blocks = make([]*common.DirectoryBlock, len(dBlocks), len(dBlocks)+1)

	for i := 0; i < len(dBlocks); i = i + 1 {
		if dBlocks[i].Header.BlockHeight != uint32(i) {
			panic("Error in initializing dChain:" + dchain.ChainID.String())
		}
		dBlocks[i].Chain = dchain
		dBlocks[i].IsSealed = true
		dBlocks[i].IsSavedInDB = true
		dchain.Blocks[i] = &dBlocks[i]
	}

	// double check the block ids
	for i := 0; i < len(dchain.Blocks); i = i + 1 {
		if uint32(i) != dchain.Blocks[i].Header.BlockHeight {
			panic(errors.New("BlockID does not equal index for chain:" + dchain.ChainID.String() + " block:" + fmt.Sprintf("%v", dchain.Blocks[i].Header.BlockHeight)))
		}
	}

	//Create an empty block and append to the chain
	if len(dchain.Blocks) == 0 {
		dchain.NextBlockHeight = 0
		dchain.NextBlock, _ = common.CreateDBlock(dchain, nil, 10)
		// Update dir block height cache in db
		h, _ := common.HexToHash(common.GENESIS_DIR_BLOCK_HASH)
		db.UpdateBlockHeightCache(0, h)
	} else {
		dchain.NextBlockHeight = uint32(len(dchain.Blocks))
		dchain.NextBlock, _ = common.CreateDBlock(dchain, dchain.Blocks[len(dchain.Blocks)-1], 10)
		// Update dir block height cache in db
		db.UpdateBlockHeightCache(dchain.NextBlockHeight-1, dchain.NextBlock.Header.PrevBlockHash)
	}

	exportDChain(dchain)

	//Double check the sealed flag
	if dchain.NextBlock.IsSealed == true {
		panic("dchain.Blocks[dchain.NextBlockID].IsSealed for chain:" + dchain.ChainID.String())
	}

}

func initECChain() {

	eCreditMap = make(map[string]int32)

	//Initialize the Entry Credit Chain ID
	ecchain = common.NewECChain()

	// get all ecBlocks from db
	ecBlocks, _ := db.FetchAllECBlocks()
	sort.Sort(util.ByECBlockIDAccending(ecBlocks))

	for i, v := range ecBlocks {
		if v.Header.DBHeight != uint32(i) {
			panic("Error in initializing dChain:" + ecchain.ChainID.String() + " DBHeight:" + strconv.Itoa(int(v.Header.DBHeight)) + " i:" + strconv.Itoa(i))
		}

		// Calculate the EC balance for each account
		initializeECreditMap(&v)
	}

	//Create an empty block and append to the chain
	if len(ecBlocks) == 0 || dchain.NextBlockHeight == 0 {
		ecchain.NextBlockHeight = 0
		ecchain.NextBlock = common.NewECBlock()
	} else {
		// Entry Credit Chain should have the same height as the dir chain
		ecchain.NextBlockHeight = dchain.NextBlockHeight
		ecchain.NextBlock = common.NextECBlock(&ecBlocks[ecchain.NextBlockHeight-1])
	}

	// create a backup copy before processing entries
	copyCreditMap(eCreditMap, eCreditMapBackup)
	exportECChain(ecchain)

	// ONly for debugging
	//printCChain()
	//printCreditMap()
	//printPaidEntryMap()

}

func initAChain() {

	//Initialize the Admin Chain ID
	achain = new(common.AdminChain)
	achain.ChainID = new(common.Hash)
	achain.ChainID.SetBytes(common.ADMIN_CHAINID)

	// get all aBlocks from db
	aBlocks, _ := db.FetchAllABlocks()
	sort.Sort(util.ByABlockIDAccending(aBlocks))

	fmt.Printf("initAChain: aBlocks=%s\n", spew.Sdump(aBlocks))

	// double check the block ids
	for i := 0; i < len(aBlocks); i = i + 1 {
		//if uint32(i) != aBlocks[i].Header.DBHeight {
		//	panic(errors.New("BlockID does not equal index for chain:" + achain.ChainID.String() + " block:" + fmt.Sprintf("%v", aBlocks[i].Header.DBHeight)))
		//}
	}

	//Create an empty block and append to the chain
	if len(aBlocks) == 0 || dchain.NextBlockHeight == 0 {
		achain.NextBlockHeight = 0
		achain.NextBlock, _ = common.CreateAdminBlock(achain, nil, 10)

	} else {
		// Entry Credit Chain should have the same height as the dir chain
		achain.NextBlockHeight = dchain.NextBlockHeight
		achain.NextBlock, _ = common.CreateAdminBlock(achain, &aBlocks[achain.NextBlockHeight-1], 10)
	}

	exportAChain(achain)

}

func initSCChain() {

	//Initialize the Admin Chain ID
	scchain = new(common.SCChain)
	scchain.ChainID = new(common.Hash)
	scchain.ChainID.SetBytes(sc.FACTOID_CHAINID)

	// get all aBlocks from db
	FBlocks, _ := db.FetchAllFBlocks()
	sort.Sort(util.ByFBlockIDAccending(FBlocks))

	// double check the block ids
	for i := 0; i < len(FBlocks); i = i + 1 {
		//if uint32(i) != aBlocks[i].Header.DBHeight {
		//	panic(errors.New("BlockID does not equal index for chain:" + achain.ChainID.String() + " block:" + fmt.Sprintf("%v", aBlocks[i].Header.DBHeight)))
		//}
	}

	//Create an empty block and append to the chain
	if len(FBlocks) == 0 || dchain.NextBlockHeight == 0 {
		scchain.NextBlockHeight = 0

		// THIS IS IN TWO PLACES HERE! THEY NEED TO MATCH!
		scchain.NextBlock = block.GetGenesisBlock(1000000, 10, 200000000000)
		data, _ := scchain.NextBlock.MarshalBinary()
		fmt.Println("\n\n ", common.Sha(data).String(), "\n\n")
	} else {
		// Entry Credit Chain should have the same height as the dir chain
		scchain.NextBlockHeight = dchain.NextBlockHeight
		scchain.NextBlock = block.NewFBlock(FactoshisPerCredit, dchain.NextBlockHeight)
	}

	exportSCChain(scchain)

}

func initEChains() {

	chainIDMap = make(map[string]*common.EChain)

	chains, err := db.FetchAllChains()

	if err != nil {
		panic(err)
	}

	for _, chain := range chains {
		var newChain = chain
		chainIDMap[newChain.ChainID.String()] = &newChain
		exportEChain(&chain)
	}

}

func initializeECreditMap(block *common.ECBlock) {
	for _, entry := range block.Body.Entries {
		// Only process: ECIDChainCommit, ECIDEntryCommit, ECIDBalanceIncrease
		switch entry.ECID() {
		case common.ECIDChainCommit:
			e := entry.(*common.CommitChain)
			eCreditMap[string(e.ECPubKey[:])] += int32(e.Credits)
		case common.ECIDEntryCommit:
			e := entry.(*common.CommitEntry)
			eCreditMap[string(e.ECPubKey[:])] += int32(e.Credits)
		case common.ECIDBalanceIncrease:
			e := entry.(*common.IncreaseBalance)
			eCreditMap[string(e.ECPubKey[:])] += int32(e.Credits)
		}
	}
}

// Initialize server private key and server public key for milestone 1
func initServerKeys() {
	if nodeMode == common.SERVER_NODE {
		var err error
		serverPrivKey, err = common.NewPrivateKeyFromHex(serverPrivKeyHex)
		if err != nil {
			panic("Cannot parse Server Private Key from configuration file: " + err.Error())
		}
	} else {
		serverPubKey = common.PubKeyFromString(common.SERVER_PUB_KEY)
	}
}

func initProcessListMgr() {
	plMgr = consensus.NewProcessListMgr(dchain.NextBlockHeight, 1, 10)

}