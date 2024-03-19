package hermez_db

import (
	"fmt"
	"strconv"

	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"

	dstypes "github.com/ledgerwatch/erigon/zk/datastream/types"
	"github.com/ledgerwatch/erigon/zk/types"
	"github.com/ledgerwatch/log/v3"
)

const L1VERIFICATIONS = "hermez_l1Verifications"                   // l1blockno, batchno -> l1txhash
const L1SEQUENCES = "hermez_l1Sequences"                           // l1blockno, batchno -> l1txhash
const FORKIDS = "hermez_forkIds"                                   // batchNo -> forkId
const FORKID_BLOCK = "hermez_forkIdBlock"                          // forkId -> startBlock
const BLOCKBATCHES = "hermez_blockBatches"                         // l2blockno -> batchno
const GLOBAL_EXIT_ROOTS = "hermez_globalExitRootsSaved"            // GER -> true
const BLOCK_GLOBAL_EXIT_ROOTS = "hermez_globalExitRoots"           // l2blockno -> GER
const GLOBAL_EXIT_ROOTS_BATCHES = "hermez_globalExitRoots_batches" // batchkno -> GER
const TX_PRICE_PERCENTAGE = "hermez_txPricePercentage"             // txHash -> txPricePercentage
const STATE_ROOTS = "hermez_stateRoots"                            // l2blockno -> stateRoot
const L1_INFO_TREE_UPDATES = "l1_info_tree_updates"                // index -> L1InfoTreeUpdate
const BLOCK_L1_INFO_TREE_INDEX = "block_l1_info_tree_index"        // block number -> l1 info tree index
const L1_INJECTED_BATCHES = "l1_injected_batches"                  // index increasing by 1 -> injected batch for the start of the chain
const BLOCK_INFO_ROOTS = "block_info_roots"                        // block number -> block info root hash
const BATCH_WITNESS = "batch_witness"                              // batch witness -> witness

type HermezDb struct {
	tx kv.RwTx
	*HermezDbReader
}

// HermezDbReader represents a reader for the HermezDb database.  It has no write functions and is embedded into the
// HermezDb type for read operations.
type HermezDbReader struct {
	tx kv.Tx
}

func NewHermezDbReader(tx kv.Tx) *HermezDbReader {
	return &HermezDbReader{tx}
}

func NewHermezDb(tx kv.RwTx) *HermezDb {
	db := &HermezDb{tx: tx}
	db.HermezDbReader = NewHermezDbReader(tx)

	return db
}

func CreateHermezBuckets(tx kv.RwTx) error {
	tables := []string{
		L1VERIFICATIONS,
		L1SEQUENCES,
		FORKIDS,
		FORKID_BLOCK,
		BLOCKBATCHES,
		GLOBAL_EXIT_ROOTS,
		BLOCK_GLOBAL_EXIT_ROOTS,
		GLOBAL_EXIT_ROOTS_BATCHES,
		TX_PRICE_PERCENTAGE,
		STATE_ROOTS,
		L1_INFO_TREE_UPDATES,
		BLOCK_L1_INFO_TREE_INDEX,
		L1_INJECTED_BATCHES,
		BLOCK_INFO_ROOTS,
		BATCH_WITNESS,
	}
	for _, t := range tables {
		if err := tx.CreateBucket(t); err != nil {
			return err
		}
	}
	return nil
}

func (db *HermezDbReader) GetBatchNoByL2Block(l2BlockNo uint64) (uint64, error) {
	c, err := db.tx.Cursor(BLOCKBATCHES)
	if err != nil {
		return 0, err
	}
	defer c.Close()

	k, v, err := c.Seek(Uint64ToBytes(l2BlockNo))
	if err != nil {
		return 0, err
	}

	if k == nil {
		return 0, nil
	}

	if BytesToUint64(k) != l2BlockNo {
		return 0, nil
	}

	return BytesToUint64(v), nil
}

func (db *HermezDbReader) GetL2BlockNosByBatch(batchNo uint64) ([]uint64, error) {
	// TODO: not the most efficient way of doing this
	c, err := db.tx.Cursor(BLOCKBATCHES)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	var blockNos []uint64
	var k, v []byte

	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			break
		}
		if BytesToUint64(v) == batchNo {
			blockNos = append(blockNos, BytesToUint64(k))
		}
	}

	return blockNos, err
}

func (db *HermezDbReader) GetLatestDownloadedBatchNo() (uint64, error) {
	c, err := db.tx.Cursor(BLOCKBATCHES)
	if err != nil {
		return 0, err
	}
	defer c.Close()

	_, v, err := c.Last()
	if err != nil {
		return 0, err
	}
	return BytesToUint64(v), nil

}

func (db *HermezDbReader) GetHighestBlockInBatch(batchNo uint64) (uint64, error) {
	blocks, err := db.GetL2BlockNosByBatch(batchNo)
	if err != nil {
		return 0, err
	}

	max := uint64(0)
	for _, block := range blocks {
		if block > max {
			max = block
		}
	}

	return max, nil
}

func (db *HermezDbReader) GetHighestVerifiedBlockNo() (uint64, error) {
	v, err := db.GetLatestVerification()
	if err != nil {
		return 0, err
	}

	if v == nil {
		return 0, nil
	}

	blockNo, err := db.GetHighestBlockInBatch(v.BatchNo)
	if err != nil {
		return 0, err
	}

	return blockNo, nil
}

func (db *HermezDbReader) GetVerificationByL2BlockNo(blockNo uint64) (*types.L1BatchInfo, error) {
	batchNo, err := db.GetBatchNoByL2Block(blockNo)
	if err != nil {
		return nil, err
	}

	return db.GetVerificationByBatchNo(batchNo)
}

func (db *HermezDbReader) GetSequenceByL1Block(l1BlockNo uint64) (*types.L1BatchInfo, error) {
	return db.getByL1Block(L1SEQUENCES, l1BlockNo)
}

func (db *HermezDbReader) GetSequenceByBatchNo(batchNo uint64) (*types.L1BatchInfo, error) {
	return db.getByBatchNo(L1SEQUENCES, batchNo)
}

func (db *HermezDbReader) GetVerificationByL1Block(l1BlockNo uint64) (*types.L1BatchInfo, error) {
	return db.getByL1Block(L1VERIFICATIONS, l1BlockNo)
}

func (db *HermezDbReader) GetVerificationByBatchNo(batchNo uint64) (*types.L1BatchInfo, error) {
	return db.getByBatchNo(L1VERIFICATIONS, batchNo)
}

func (db *HermezDbReader) getByL1Block(table string, l1BlockNo uint64) (*types.L1BatchInfo, error) {
	c, err := db.tx.Cursor(table)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	var k, v []byte
	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			return nil, err
		}

		l1Block, batchNo, err := SplitKey(k)
		if err != nil {
			return nil, err
		}

		if l1Block == l1BlockNo {
			if len(v) != 96 && len(v) != 64 {
				return nil, fmt.Errorf("invalid hash length")
			}

			l1TxHash := common.BytesToHash(v[:32])
			stateRoot := common.BytesToHash(v[32:64])

			return &types.L1BatchInfo{
				BatchNo:   batchNo,
				L1BlockNo: l1Block,
				StateRoot: stateRoot,
				L1TxHash:  l1TxHash,
			}, nil
		}
	}

	return nil, nil
}

func (db *HermezDbReader) getByBatchNo(table string, batchNo uint64) (*types.L1BatchInfo, error) {
	c, err := db.tx.Cursor(table)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	var k, v []byte
	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			return nil, err
		}

		l1Block, batch, err := SplitKey(k)
		if err != nil {
			return nil, err
		}

		if batch == batchNo {
			if len(v) != 96 && len(v) != 64 {
				return nil, fmt.Errorf("invalid hash length")
			}

			l1TxHash := common.BytesToHash(v[:32])
			stateRoot := common.BytesToHash(v[32:64])
			var l1InfoRoot common.Hash
			if len(v) > 64 {
				l1InfoRoot = common.BytesToHash(v[64:])
			}

			return &types.L1BatchInfo{
				BatchNo:    batchNo,
				L1BlockNo:  l1Block,
				StateRoot:  stateRoot,
				L1TxHash:   l1TxHash,
				L1InfoRoot: l1InfoRoot,
			}, nil
		}
	}

	return nil, nil
}

func (db *HermezDbReader) GetLatestSequence() (*types.L1BatchInfo, error) {
	return db.getLatest(L1SEQUENCES)
}

func (db *HermezDbReader) GetLatestVerification() (*types.L1BatchInfo, error) {
	return db.getLatest(L1VERIFICATIONS)
}

func (db *HermezDbReader) getLatest(table string) (*types.L1BatchInfo, error) {
	c, err := db.tx.Cursor(table)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	k, v, err := c.Last()
	if err != nil {
		return nil, err
	}

	l1BlockNo, batchNo, err := SplitKey(k)
	if err != nil {
		return nil, err
	}

	if len(v) != 96 && len(v) != 64 {
		return nil, fmt.Errorf("invalid hash length")
	}

	l1TxHash := common.BytesToHash(v[:32])
	stateRoot := common.BytesToHash(v[32:64])
	var l1InfoRoot common.Hash
	if len(v) > 64 {
		l1InfoRoot = common.BytesToHash(v[64:])
	}

	return &types.L1BatchInfo{
		BatchNo:    batchNo,
		L1BlockNo:  l1BlockNo,
		L1TxHash:   l1TxHash,
		StateRoot:  stateRoot,
		L1InfoRoot: l1InfoRoot,
	}, nil
}

func (db *HermezDb) WriteSequence(l1BlockNo, batchNo uint64, l1TxHash, stateRoot common.Hash) error {
	val := append(l1TxHash.Bytes(), stateRoot.Bytes()...)
	return db.tx.Put(L1SEQUENCES, ConcatKey(l1BlockNo, batchNo), val)
}

func (db *HermezDb) WriteVerification(l1BlockNo, batchNo uint64, l1TxHash common.Hash, stateRoot common.Hash) error {
	return db.tx.Put(L1VERIFICATIONS, ConcatKey(l1BlockNo, batchNo), append(l1TxHash.Bytes(), stateRoot.Bytes()...))
}

func (db *HermezDb) WriteBlockBatch(l2BlockNo, batchNo uint64) error {
	return db.tx.Put(BLOCKBATCHES, Uint64ToBytes(l2BlockNo), Uint64ToBytes(batchNo))
}

func (db *HermezDb) WriteGlobalExitRoot(ger common.Hash) error {
	return db.tx.Put(GLOBAL_EXIT_ROOTS, ger.Bytes(), []byte{1})
}

func (db *HermezDbReader) GetGlobalExitRoot(ger common.Hash) (bool, error) {
	bytes, err := db.tx.GetOne(GLOBAL_EXIT_ROOTS, ger.Bytes())
	if err != nil {
		return false, err
	}
	return len(bytes) > 0, nil
}

func (db *HermezDb) DeleteGlobalExitRoots(gers *[]common.Hash) error {
	for _, ger := range *gers {
		err := db.tx.Delete(GLOBAL_EXIT_ROOTS, ger.Bytes())
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *HermezDb) WriteBlockGlobalExitRoot(l2BlockNo uint64, ger, l1BlockHash common.Hash) error {
	key := ConcatGerKey(l2BlockNo, l1BlockHash)
	return db.tx.Put(BLOCK_GLOBAL_EXIT_ROOTS, key, ger.Bytes())
}

func (db *HermezDbReader) GetBlockGlobalExitRoot(l2BlockNo uint64) (common.Hash, common.Hash, error) {
	var h common.Hash
	var l1BlockHash common.Hash

	blockNoBytes := Uint64ToBytes(l2BlockNo)
	err := db.tx.ForPrefix(BLOCK_GLOBAL_EXIT_ROOTS, blockNoBytes, func(k, v []byte) error {
		h = common.BytesToHash(v)
		if len(k) == 40 {
			l1BlockHash = common.BytesToHash(k[8:])
		}
		return nil
	})
	if err != nil {
		return common.Hash{}, common.Hash{}, err
	}

	return h, l1BlockHash, nil
}

// from and to are inclusive
func (db *HermezDbReader) GetBlockGlobalExitRoots(fromBlockNo, toBlockNo uint64) ([]common.Hash, []common.Hash, error) {
	c, err := db.tx.Cursor(BLOCK_GLOBAL_EXIT_ROOTS)
	if err != nil {
		return nil, nil, err
	}
	defer c.Close()

	var gers, l1BlockHashes []common.Hash

	var k, v []byte

	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			return nil, nil, err
		}
		CurrentBlockNumber := BytesToUint64(k)
		if CurrentBlockNumber >= fromBlockNo && CurrentBlockNumber <= toBlockNo {
			h := common.BytesToHash(v)
			gers = append(gers, h)
			if len(k) == 40 {
				l1BlockHash := common.BytesToHash(k[8:])
				l1BlockHashes = append(l1BlockHashes, l1BlockHash)
			}
		}
	}

	return gers, l1BlockHashes, nil
}

func (db *HermezDb) WriteBatchGlobalExitRoot(batchNumber uint64, ger dstypes.GerUpdate) error {
	return db.tx.Put(GLOBAL_EXIT_ROOTS_BATCHES, Uint64ToBytes(batchNumber), ger.EncodeToBytes())
}

func (db *HermezDbReader) GetBatchGlobalExitRoots(fromBatchNum, toBatchNum uint64) ([]*dstypes.GerUpdate, error) {
	c, err := db.tx.Cursor(GLOBAL_EXIT_ROOTS_BATCHES)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	var gers []*dstypes.GerUpdate
	var k, v []byte

	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			break
		}
		currentBatchNo := BytesToUint64(k)
		if currentBatchNo >= fromBatchNum && currentBatchNo <= toBatchNum {
			gerUpdate, err := dstypes.DecodeGerUpdate(v)
			if err != nil {
				return nil, err
			}
			gers = append(gers, gerUpdate)
		}
	}

	return gers, err
}

func (db *HermezDbReader) GetBatchGlobalExitRoot(batchNum uint64) (*dstypes.GerUpdate, error) {
	gerUpdateBytes, err := db.tx.GetOne(GLOBAL_EXIT_ROOTS_BATCHES, Uint64ToBytes(batchNum))
	if err != nil {
		return nil, err
	}
	if len(gerUpdateBytes) == 0 {
		// no ger update for this batch
		return nil, nil
	}
	gerUpdate, err := dstypes.DecodeGerUpdate(gerUpdateBytes)
	if err != nil {
		return nil, err
	}
	return gerUpdate, nil
}

func (db *HermezDb) DeleteBatchGlobalExitRoots(fromBatchNum uint64) error {
	c, err := db.tx.Cursor(GLOBAL_EXIT_ROOTS_BATCHES)
	if err != nil {
		return err
	}
	defer c.Close()

	k, _, err := c.Last()
	if err != nil {
		return err
	}

	for i := fromBatchNum; i <= BytesToUint64(k); i++ {
		err := db.tx.Delete(GLOBAL_EXIT_ROOTS_BATCHES, Uint64ToBytes(i))
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *HermezDb) DeleteBlockGlobalExitRoots(fromBlockNum, toBlockNum uint64) error {
	c, err := db.tx.Cursor(BLOCK_GLOBAL_EXIT_ROOTS)
	if err != nil {
		return err
	}
	defer c.Close()

	var k []byte
	var keys [][]byte
	for k, _, err = c.First(); k != nil; k, _, err = c.Next() {
		if err != nil {
			break
		}
		blockNum := BytesToUint64(k[:8])
		if blockNum >= fromBlockNum && blockNum <= toBlockNum {
			keys = append(keys, k)
		}
	}
	for _, key := range keys {
		err := db.tx.Delete(BLOCK_GLOBAL_EXIT_ROOTS, key)
		if err != nil {
			return err
		}
	}

	return nil
}

// from and to are inclusive
func (db *HermezDb) DeleteBlockBatches(fromBlockNum, toBlockNum uint64) error {
	for i := fromBlockNum; i <= toBlockNum; i++ {
		err := db.tx.Delete(BLOCKBATCHES, Uint64ToBytes(i))
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *HermezDbReader) GetForkId(batchNo uint64) (uint64, error) {
	c, err := db.tx.Cursor(FORKIDS)
	if err != nil {
		return 0, err
	}
	defer c.Close()

	var forkId uint64 = 0
	var k, v []byte

	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			break
		}
		currentBatchNo := BytesToUint64(k)
		if currentBatchNo <= batchNo {
			forkId = BytesToUint64(v)
		} else {
			break
		}
	}

	return forkId, err
}

func (db *HermezDb) WriteForkId(batchNo, forkId uint64) error {
	return db.tx.Put(FORKIDS, Uint64ToBytes(batchNo), Uint64ToBytes(forkId))
}

func (db *HermezDbReader) GetForkIdBlock(forkId uint64) (uint64, error) {
	c, err := db.tx.Cursor(FORKID_BLOCK)
	if err != nil {
		return 0, err
	}
	defer c.Close()

	var blockNum uint64 = 0
	var k, v []byte

	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			break
		}
		currentForkId := BytesToUint64(k)
		if currentForkId == forkId {
			blockNum = BytesToUint64(v)
			log.Info(fmt.Sprintf("[HermezDbReader] Got block num %d for forkId %d", blockNum, forkId))
			break
		} else {
			continue
		}
	}

	return blockNum, err
}

func (db *HermezDb) DeleteForkIdBlock(fromBlockNo, toBlockNo uint64) error {
	c, err := db.tx.Cursor(FORKID_BLOCK)
	if err != nil {
		return err
	}
	defer c.Close()

	var forkIds []uint64
	var k, v []byte
	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			break
		}

		blockNum := BytesToUint64(v)
		if blockNum >= fromBlockNo && blockNum <= toBlockNo {
			currentForkId := BytesToUint64(k)
			forkIds = append(forkIds, currentForkId)
		}
	}

	if err != nil {
		return err
	}

	for _, forkId := range forkIds {
		err := db.tx.Delete(FORKID_BLOCK, Uint64ToBytes(forkId))
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *HermezDb) WriteForkIdBlockOnce(forkId, blockNum uint64) error {
	tempBlockNum, err := db.GetForkIdBlock(forkId)
	if err != nil {
		log.Error(fmt.Sprintf("[HermezDb] Error getting forkIdBlock: %v", err))
		return err
	}
	if tempBlockNum != 0 {
		log.Error(fmt.Sprintf("[HermezDb] Fork id block already exists: %d, block:%v, set db failed.", forkId, tempBlockNum))
		return nil
	}
	return db.tx.Put(FORKID_BLOCK, Uint64ToBytes(forkId), Uint64ToBytes(blockNum))
}

func (db *HermezDb) DeleteForkIds(fromBatchNum, toBatchNum uint64) error {
	for i := fromBatchNum; i <= toBatchNum; i++ {
		err := db.tx.Delete(FORKIDS, Uint64ToBytes(i))
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *HermezDb) WriteEffectiveGasPricePercentage(txHash common.Hash, txPricePercentage uint8) error {
	return db.tx.Put(TX_PRICE_PERCENTAGE, txHash.Bytes(), Uint8ToBytes(txPricePercentage))
}

func (db *HermezDbReader) GetEffectiveGasPricePercentage(txHash common.Hash) (uint8, error) {
	data, err := db.tx.GetOne(TX_PRICE_PERCENTAGE, txHash.Bytes())
	if err != nil {
		return 0, err
	}

	return BytesToUint8(data), nil
}

func (db *HermezDb) DeleteEffectiveGasPricePercentages(txHashes *[]common.Hash) error {
	for _, txHash := range *txHashes {
		err := db.tx.Delete(TX_PRICE_PERCENTAGE, txHash.Bytes())
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *HermezDb) WriteStateRoot(l2BlockNo uint64, rpcRoot common.Hash) error {
	return db.tx.Put(STATE_ROOTS, Uint64ToBytes(l2BlockNo), rpcRoot.Bytes())
}

func (db *HermezDbReader) GetStateRoot(l2BlockNo uint64) (common.Hash, error) {
	data, err := db.tx.GetOne(STATE_ROOTS, Uint64ToBytes(l2BlockNo))
	if err != nil {
		return common.Hash{}, err
	}

	return common.BytesToHash(data), nil
}

func (db *HermezDb) DeleteStateRoots(fromBlockNo, toBlockNo uint64) error {
	for i := fromBlockNo; i <= toBlockNo; i++ {
		if err := db.tx.Delete(STATE_ROOTS, Uint64ToBytes(i)); err != nil {
			return err
		}
	}
	return nil
}

func (db *HermezDb) WriteL1InfoTreeUpdate(update *types.L1InfoTreeUpdate) error {
	marshalled := update.Marshall()
	idx := Uint64ToBytes(update.Index)
	return db.tx.Put(L1_INFO_TREE_UPDATES, idx, marshalled)
}

func (db *HermezDbReader) GetL1InfoTreeUpdate(idx uint64) (*types.L1InfoTreeUpdate, error) {
	data, err := db.tx.GetOne(L1_INFO_TREE_UPDATES, Uint64ToBytes(idx))
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	update := &types.L1InfoTreeUpdate{}
	update.Unmarshall(data)
	return update, nil
}

func (db *HermezDbReader) GetLatestL1InfoTreeUpdate() (*types.L1InfoTreeUpdate, bool, error) {
	cursor, err := db.tx.Cursor(L1_INFO_TREE_UPDATES)
	if err != nil {
		return nil, false, err
	}
	defer cursor.Close()

	count, err := cursor.Count()
	if err != nil {
		return nil, false, err
	}
	if count == 0 {
		return nil, false, nil
	}

	_, v, err := cursor.Last()
	if err != nil {
		return nil, false, err
	}
	if len(v) == 0 {
		return nil, false, nil
	}

	result := &types.L1InfoTreeUpdate{}
	result.Unmarshall(v)
	return result, true, nil
}

func (db *HermezDb) WriteBlockL1InfoTreeIndex(blockNumber uint64, l1Index uint64) error {
	k := Uint64ToBytes(blockNumber)
	v := Uint64ToBytes(l1Index)
	return db.tx.Put(BLOCK_L1_INFO_TREE_INDEX, k, v)
}

func (db *HermezDbReader) GetBlockL1InfoTreeIndex(blockNumber uint64) (uint64, error) {
	v, err := db.tx.GetOne(BLOCK_L1_INFO_TREE_INDEX, Uint64ToBytes(blockNumber))
	if err != nil {
		return 0, err
	}
	return BytesToUint64(v), nil
}

func (db *HermezDb) WriteL1InjectedBatch(batch *types.L1InjectedBatch) error {
	var nextIndex uint64 = 0

	// get the next index for the write
	cursor, err := db.tx.Cursor(L1_INJECTED_BATCHES)
	if err != nil {
		return err
	}

	count, err := cursor.Count()
	if err != nil {
		return err
	}

	if count > 0 {
		nextIndex = count + 1
	}

	k := Uint64ToBytes(nextIndex)
	v := batch.Marshall()
	return db.tx.Put(L1_INJECTED_BATCHES, k, v)
}

func (db *HermezDbReader) GetL1InjectedBatch(index uint64) (*types.L1InjectedBatch, error) {
	k := Uint64ToBytes(index)
	v, err := db.tx.GetOne(L1_INJECTED_BATCHES, k)
	if err != nil {
		return nil, err
	}
	ib := new(types.L1InjectedBatch)
	err = ib.Unmarshall(v)
	if err != nil {
		return nil, err
	}
	return ib, nil
}

func (db *HermezDb) WriteBlockInfoRoot(blockNumber uint64, root common.Hash) error {
	k := Uint64ToBytes(blockNumber)
	return db.tx.Put(BLOCK_INFO_ROOTS, k, root.Bytes())
}

func (db *HermezDbReader) GetBlockInfoRoot(blockNumber uint64) (common.Hash, error) {
	k := Uint64ToBytes(blockNumber)
	data, err := db.tx.GetOne(BLOCK_INFO_ROOTS, k)
	if err != nil {
		return common.Hash{}, err
	}
	res := common.BytesToHash(data)
	return res, nil
}

func (db *HermezDbReader) GetWitnessByBatchNo(batchNo uint64) ([]byte, error) {
	w, err := ReadChunks(db.tx, BATCH_WITNESS, Uint64ToBytes(batchNo))
	if err != nil {
		return nil, err
	}

	return w, nil
}

func (db *HermezDb) WriteWitnessByBatchNo(batchNo uint64, w []byte) error {
	return WriteChunks(db.tx, BATCH_WITNESS, Uint64ToBytes(batchNo), w)
}

func (db *HermezDb) DeleteWitnessByBatchNo(batchNo uint64) error {
	highestBlock, err := db.GetHighestBlockInBatch(batchNo)
	if err != nil {
		return err
	}
	// If highest block is 0, it implies that batch i is not present
	if highestBlock == 0 {
		return nil
	}

	return DeleteChunks(db.tx, BATCH_WITNESS, Uint64ToBytes(batchNo))
}

func (db *HermezDb) DeleteWitnessByBatchRange(fromBatch uint64, toBatch uint64) error {
	for i := fromBatch; i <= toBatch; i++ {
		err := db.DeleteWitnessByBatchNo(i)
		if err != nil {
			return err
		}
	}

	return nil
}

const chunkSize = 100000 // 100KB

func WriteChunks(tx kv.RwTx, tableName string, key []byte, valueBytes []byte) error {
	// Split the valueBytes into chunks and write each chunk
	for i := 0; i < len(valueBytes); i += chunkSize {
		end := i + chunkSize
		if end > len(valueBytes) {
			end = len(valueBytes)
		}
		chunk := valueBytes[i:end]
		chunkKey := append(key, []byte("_chunk_"+strconv.Itoa(i/chunkSize))...)

		// Write each chunk to the KV store
		if err := tx.Put(tableName, chunkKey, chunk); err != nil {
			return err
		}
	}

	return nil
}

func ReadChunks(tx kv.Tx, tableName string, key []byte) ([]byte, error) {
	// Initialize a buffer to store the concatenated chunks
	var result []byte

	// Retrieve and concatenate each chunk
	for i := 0; ; i++ {
		chunkKey := append(key, []byte("_chunk_"+strconv.Itoa(i))...)
		chunk, err := tx.GetOne(tableName, chunkKey)
		if err != nil {
			return nil, err
		}

		// Check if this is the last chunk
		if len(chunk) == 0 {
			break
		}

		// Append the chunk to the result
		result = append(result, chunk...)
	}

	return result, nil
}

func DeleteChunks(tx kv.RwTx, tableName string, key []byte) error {
	for i := 0; ; i++ {
		chunkKey := append(key, []byte("_chunk_"+strconv.Itoa(i))...)
		chunk, err := tx.GetOne(tableName, chunkKey)
		if err != nil {
			return err
		}

		err = tx.Delete(tableName, chunkKey)
		if err != nil {
			return err
		}

		// Check if this is the last chunk
		if len(chunk) == 0 {
			break
		}

	}

	return nil
}
