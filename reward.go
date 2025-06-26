package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

func getBlockRewards(ctx context.Context, chainId uint64, client *ethclient.Client, block *types.Block, blockReceipts []*types.Receipt) (map[common.Address]*big.Int, map[common.Hash]*big.Int) {
	var result []map[string]any
	blockHeader := block.Header()

	retryUntilSuccessOrContextDone(ctx, func(ctx context.Context) error {
		err := client.Client().CallContext(ctx, &result, "trace_block", fmt.Sprintf("0x%x", blockHeader.Number.Uint64()))
		if err != nil {
			if strings.Contains(err.Error(), "trace_block") && strings.Contains(err.Error(), "does not exist") {
				log.Printf("⚠️ trace_block não está disponível no RPC — recompensas ignoradas")
				result = nil
				return nil
			}
			log.Printf("Erro no trace_block: %v", err)
			return err
		}
		return nil
	}, "trace_block")

	if result == nil {
		return make(map[common.Address]*big.Int), make(map[common.Hash]*big.Int)
	}

	rewardsByMiner := make(map[common.Address]*big.Int)
	rewardsByUncleBlock := make(map[common.Hash]*big.Int)
	uncleHeaders := make(map[*types.Header]struct{})
	for _, uncle := range block.Uncles() {
		uncleHeaders[uncle] = struct{}{}
	}

	for _, data := range result {
		action := data["action"].(map[string]any)
		rewardType, isRewardAction := action["rewardType"]
		if !isRewardAction {
			continue
		}
		author := common.HexToAddress(action["author"].(string))
		reward, success := new(big.Int).SetString(action["value"].(string), 0)
		if !success {
			panic("programming error: reward inválido")
		}
		switch rewardType {
		case "block":
			miner := getMiner(ctx, chainId, client, blockHeader)
			if miner != author {
				panic("programming error: miner != author")
			}
			if rewardsByMiner[miner] == nil {
				rewardsByMiner[miner] = new(big.Int)
			}
			rewardsByMiner[miner].Add(rewardsByMiner[miner], reward)

		case "uncle":
			for uncle := range uncleHeaders {
				if getMiner(ctx, chainId, client, uncle) == author {
					rewardsByUncleBlock[uncle.Hash()] = reward
					if rewardsByMiner[author] == nil {
						rewardsByMiner[author] = new(big.Int)
					}
					rewardsByMiner[author].Add(rewardsByMiner[author], reward)
					delete(uncleHeaders, uncle)
					break
				}
			}
		default:
			panic("programming error: rewardType desconhecido")
		}
	}

	return rewardsByMiner, rewardsByUncleBlock
}
