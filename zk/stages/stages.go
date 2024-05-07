package stages

import (
	"context"

	"github.com/ledgerwatch/log/v3"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/wrap"

	stages "github.com/ledgerwatch/erigon/eth/stagedsync"
	stages2 "github.com/ledgerwatch/erigon/eth/stagedsync/stages"
)

func SequencerZkStages(
	ctx context.Context,
	l1InfoTreeCfg L1SequencerSyncCfg,
	dataStreamCatchupCfg DataStreamCatchupCfg,
	sequencerInterhashesCfg SequencerInterhashesCfg,
	exec SequenceBlockCfg,
	hashState stages.HashStateCfg,
	zkInterHashesCfg ZkInterHashesCfg,
	sequencerExecutorVerifyCfg SequencerExecutorVerifyCfg,
	history stages.HistoryCfg,
	logIndex stages.LogIndexCfg,
	callTraces stages.CallTracesCfg,
	txLookup stages.TxLookupCfg,
	finish stages.FinishCfg,
	test bool,
) []*stages.Stage {
	return []*stages.Stage{
		{
			ID:          stages2.L1Syncer,
			Description: "L1 Sequencer Sync Updates",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return SpawnL1SequencerSyncStage(s, u, txc.Tx, l1InfoTreeCfg, ctx, firstCycle, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return UnwindL1SequencerSyncStage(u, txc.Tx, l1InfoTreeCfg, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return PruneL1SequencerSyncStage(p, tx, l1InfoTreeCfg, ctx)
			},
		},
		{
			/*
				TODO:
				we need to have another "execution" stage, that takes data from the txpool instead of from headers/bodies.

				this "execution" stage should, in fact, write the following:
				* block headers
				* block bodies
				* batches

				currently it could be hard-coded to 1 batch -contains-> 1 block -contains-> 1 tx
					+------------+
					|  Batch #1  |
					+------------+
					| +--------+ |
					| |Block #1| |
					| |        | |
					| | +Tx #1 | |
					| +--------+ |
					+------------+
				later it should take both the gas limit of the block and the zk counters limit

				it should also generate a retainlist for the future batch witness generation
			*/
			ID:          stages2.Execution,
			Description: "Sequence transactions",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return SpawnSequencingStage(s, u, txc.Tx, 0, ctx, exec, firstCycle)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return UnwindSequenceExecutionStage(u, s, txc.Tx, ctx, exec, firstCycle)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return PruneSequenceExecutionStage(p, tx, exec, ctx, firstCycle)
			},
		},
		{
			ID:          stages2.IntermediateHashes,
			Description: "Sequencer Intermediate Hashes",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return SpawnSequencerInterhashesStage(s, u, txc.Tx, ctx, sequencerInterhashesCfg, firstCycle)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return UnwindSequencerInterhashsStage(u, s, txc.Tx, ctx, sequencerInterhashesCfg, firstCycle)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return PruneSequencerInterhashesStage(p, tx, sequencerInterhashesCfg, ctx, firstCycle)
			},
		},
		{
			ID:          stages2.SequenceExecutorVerify,
			Description: "Sequencer, check batch with legacy executor",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return SpawnSequencerExecutorVerifyStage(s, u, txc.Tx, ctx, sequencerExecutorVerifyCfg, firstCycle)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return UnwindSequencerExecutorVerifyStage(u, s, txc.Tx, ctx, sequencerExecutorVerifyCfg, firstCycle)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return PruneSequencerExecutorVerifyStage(p, tx, sequencerExecutorVerifyCfg, ctx, firstCycle)
			},
		},
		{
			ID:          stages2.HashState,
			Description: "Hash the key in the state",
			Disabled:    false,
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnHashStateStage(s, txc.Tx, hashState, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindHashStateStage(u, s, txc.Tx, hashState, ctx, logger)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneHashStateStage(p, tx, hashState, ctx)
			},
		},
		{
			ID:                  stages2.CallTraces,
			Description:         "Generate call traces index",
			DisabledDescription: "Work In Progress",
			Disabled:            false,
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnCallTraces(s, txc.Tx, callTraces, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindCallTraces(u, s, txc.Tx, callTraces, ctx, logger)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneCallTraces(p, tx, callTraces, ctx, logger)
			},
		},
		{
			ID:          stages2.AccountHistoryIndex,
			Description: "Generate account history index",
			Disabled:    false,
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnAccountHistoryIndex(s, txc.Tx, history, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindAccountHistoryIndex(u, s, txc.Tx, history, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneAccountHistoryIndex(p, tx, history, ctx, logger)
			},
		},
		{
			ID:          stages2.StorageHistoryIndex,
			Description: "Generate storage history index",
			Disabled:    false,
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnStorageHistoryIndex(s, txc.Tx, history, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindStorageHistoryIndex(u, s, txc.Tx, history, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneStorageHistoryIndex(p, tx, history, ctx, logger)
			},
		},
		{
			ID:          stages2.LogIndex,
			Description: "Generate receipt logs index",
			Disabled:    false,
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnLogIndex(s, txc.Tx, logIndex, ctx, 0, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindLogIndex(u, s, txc.Tx, logIndex, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneLogIndex(p, tx, logIndex, ctx, logger)
			},
		},
		{
			ID:          stages2.TxLookup,
			Description: "Generate tx lookup index",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnTxLookup(s, txc.Tx, 0 /* toBlock */, txLookup, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindTxLookup(u, s, txc.Tx, txLookup, ctx, logger)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneTxLookup(p, tx, txLookup, ctx, firstCycle, logger)
			},
		},
		/*
		  TODO: verify batches stage -- real executor that verifies batches
		  if it fails, we need to unwind everything up until before the bad batch
		*/
		{
			ID:          stages2.DataStream,
			Description: "Update the data stream with missing details",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return SpawnStageDataStreamCatchup(s, ctx, txc.Tx, dataStreamCatchupCfg)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return nil
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return nil
			},
		},
		{
			ID:          stages2.Finish,
			Description: "Final: update current block for the RPC API",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, _ stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.FinishForward(s, txc.Tx, finish, firstCycle)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindFinish(u, txc.Tx, finish, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneFinish(p, tx, finish, ctx)
			},
		},
	}
}

func DefaultZkStages(
	ctx context.Context,
	l1SyncerCfg L1SyncerCfg,
	batchesCfg BatchesCfg,
	dataStreamCatchupCfg DataStreamCatchupCfg,
	blockHashCfg stages.BlockHashesCfg,
	senders stages.SendersCfg,
	exec stages.ExecuteBlockCfg,
	hashState stages.HashStateCfg,
	zkInterHashesCfg ZkInterHashesCfg,
	history stages.HistoryCfg,
	logIndex stages.LogIndexCfg,
	callTraces stages.CallTracesCfg,
	txLookup stages.TxLookupCfg,
	finish stages.FinishCfg,
	test bool,
) []*stages.Stage {
	return []*stages.Stage{
		{
			ID:          stages2.L1Syncer,
			Description: "Download L1 Verifications",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				if badBlockUnwind {
					return nil
				}
				return SpawnStageL1Syncer(s, u, ctx, txc.Tx, l1SyncerCfg, firstCycle, test)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return UnwindL1SyncerStage(u, txc.Tx, l1SyncerCfg, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return PruneL1SyncerStage(p, tx, l1SyncerCfg, ctx)
			},
		},
		{
			ID:          stages2.Batches,
			Description: "Download batches",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				if badBlockUnwind {
					return nil
				}
				return SpawnStageBatches(s, u, ctx, txc.Tx, batchesCfg, firstCycle, test)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return UnwindBatchesStage(u, txc.Tx, batchesCfg, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return PruneBatchesStage(p, tx, batchesCfg, ctx)
			},
		},
		{
			ID:          stages2.BlockHashes,
			Description: "Write block hashes",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnBlockHashStage(s, txc.Tx, blockHashCfg, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindBlockHashStage(u, txc.Tx, blockHashCfg, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneBlockHashStage(p, tx, blockHashCfg, ctx)
			},
		},
		{
			ID:          stages2.Senders,
			Description: "Recover senders from tx signatures",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnRecoverSendersStage(senders, s, u, txc.Tx, 0, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindSendersStage(u, txc.Tx, senders, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneSendersStage(p, tx, senders, ctx)
			},
		},
		{
			ID:          stages2.Execution,
			Description: "Execute blocks w/o hash checks",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnExecuteBlocksStageZk(s, u, txc.Tx, 0, ctx, exec, firstCycle)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindExecutionStageZk(u, s, txc.Tx, ctx, exec, firstCycle)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneExecutionStageZk(p, tx, exec, ctx, firstCycle)
			},
		},
		{
			ID:          stages2.HashState,
			Description: "Hash the key in the state",
			Disabled:    false,
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnHashStateStage(s, txc.Tx, hashState, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindHashStateStage(u, s, txc.Tx, hashState, ctx, logger)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneHashStateStage(p, tx, hashState, ctx)
			},
		},
		{
			ID:          stages2.IntermediateHashes,
			Description: "Generate intermediate hashes and computing state root",
			Disabled:    false,
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				_, err := SpawnZkIntermediateHashesStage(s, u, txc.Tx, zkInterHashesCfg, ctx)
				return err
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return UnwindZkIntermediateHashesStage(u, s, txc.Tx, zkInterHashesCfg, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				// TODO: implement this in zk interhashes
				return nil
			},
		},
		{
			ID:                  stages2.CallTraces,
			Description:         "Generate call traces index",
			DisabledDescription: "Work In Progress",
			Disabled:            false,
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnCallTraces(s, txc.Tx, callTraces, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindCallTraces(u, s, txc.Tx, callTraces, ctx, logger)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneCallTraces(p, tx, callTraces, ctx, logger)
			},
		},
		{
			ID:          stages2.AccountHistoryIndex,
			Description: "Generate account history index",
			Disabled:    false,

			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnAccountHistoryIndex(s, txc.Tx, history, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindAccountHistoryIndex(u, s, txc.Tx, history, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneAccountHistoryIndex(p, tx, history, ctx, logger)
			},
		},
		{
			ID:          stages2.StorageHistoryIndex,
			Description: "Generate storage history index",
			Disabled:    false,
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnStorageHistoryIndex(s, txc.Tx, history, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindStorageHistoryIndex(u, s, txc.Tx, history, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneStorageHistoryIndex(p, tx, history, ctx, logger)
			},
		},
		{
			ID:          stages2.LogIndex,
			Description: "Generate receipt logs index",
			Disabled:    false,
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnLogIndex(s, txc.Tx, logIndex, ctx, 0, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindLogIndex(u, s, txc.Tx, logIndex, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneLogIndex(p, tx, logIndex, ctx, logger)
			},
		},
		{
			ID:          stages2.TxLookup,
			Description: "Generate tx lookup index",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.SpawnTxLookup(s, txc.Tx, 0 /* toBlock */, txLookup, ctx, logger)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindTxLookup(u, s, txc.Tx, txLookup, ctx, logger)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneTxLookup(p, tx, txLookup, ctx, firstCycle, logger)
			},
		},
		{
			ID:          stages2.DataStream,
			Description: "Update the data stream with missing details",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, u stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return SpawnStageDataStreamCatchup(s, ctx, txc.Tx, dataStreamCatchupCfg)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return nil
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return nil
			},
		},
		{
			ID:          stages2.Finish,
			Description: "Final: update current block for the RPC API",
			Forward: func(firstCycle bool, badBlockUnwind bool, s *stages.StageState, _ stages.Unwinder, txc wrap.TxContainer, logger log.Logger) error {
				return stages.FinishForward(s, txc.Tx, finish, firstCycle)
			},
			Unwind: func(firstCycle bool, u *stages.UnwindState, s *stages.StageState, txc wrap.TxContainer, logger log.Logger) error {
				return stages.UnwindFinish(u, txc.Tx, finish, ctx)
			},
			Prune: func(firstCycle bool, p *stages.PruneState, tx kv.RwTx, logger log.Logger) error {
				return stages.PruneFinish(p, tx, finish, ctx)
			},
		},
	}
}

var AllStagesZk = []stages2.SyncStage{
	stages2.L1Syncer,
	stages2.Batches,
	stages2.BlockHashes,
	stages2.Senders,
	stages2.Execution,
	stages2.HashState,
	stages2.IntermediateHashes,
	stages2.LogIndex,
	stages2.CallTraces,
	stages2.TxLookup,
	stages2.Finish,
}

var ZkSequencerUnwindOrder = stages.UnwindOrder{
	stages2.IntermediateHashes, // need to unwind SMT before we remove history
	stages2.Execution,
	stages2.HashState,
	stages2.CallTraces,
	stages2.AccountHistoryIndex,
	stages2.StorageHistoryIndex,
	stages2.LogIndex,
	stages2.TxLookup,
	stages2.Finish,
}

var ZkUnwindOrder = stages.UnwindOrder{
	stages2.L1Syncer,
	stages2.Batches,
	stages2.BlockHashes,
	stages2.IntermediateHashes, // need to unwind SMT before we remove history
	stages2.Execution,
	stages2.HashState,
	stages2.Senders,
	stages2.CallTraces,
	stages2.AccountHistoryIndex,
	stages2.StorageHistoryIndex,
	stages2.LogIndex,
	stages2.TxLookup,
	stages2.Finish,
}
