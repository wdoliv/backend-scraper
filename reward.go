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
	err := retryUntilSuccessOrContextDone(ctx, func(ctx context.Context) error {
		err := client.Client().CallContext(ctx, &result, "trace_block", fmt.Sprintf("0x%x", blockHeader.Number.Uint64()))
		if err != nil {
			// Se o método trace_block não existe, apenas loga e retorna nil para não falhar
			if strings.Contains(err.Error(), "trace_block") && strings.Contains(err.Error(), "does not exist") {
				log.Printf("trace_block não disponível no nó RPC, ignorando recompensas por trace_block")
				return nil
			}
			return err
		}
		return nil
	}, "trace_block")

	if err != nil {
		log.Printf("Erro na chamada trace_block: %v", err)
		// Decide aqui se quer continuar (retornando mapas vazios) ou abortar com panic/return nil
		// Vou optar por continuar com mapas vazios:
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
			panic("programming error")
		}
		switch rewardType {
		case "block":
			miner := getMiner(ctx, chainId, client, blockHeader)
			if miner != author {
				panic("programming error")
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
			panic("programming error")
		}
	}
	return rewardsByMiner, rewardsByUncleBlock
}
