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

// Assumindo que retryUntilSuccessOrContextDone tem assinatura parecida com:
// func retryUntilSuccessOrContextDone(ctx context.Context, f func(context.Context) error, desc string)

func getBlockRewards(ctx context.Context, chainId uint64, client *ethclient.Client, block *types.Block, blockReceipts []*types.Receipt) (map[common.Address]*big.Int, map[common.Hash]*big.Int) {
	var result []map[string]any
	blockHeader := block.Header()

	// Chama retry sem atribuir retorno, trata erro dentro do callback
	retryUntilSuccessOrContextDone(ctx, func(ctx context.Context) error {
		err := client.Client().CallContext(ctx, &result, "trace_block", fmt.Sprintf("0x%x", blockHeader.Number.Uint64()))
		if err != nil {
			// Se o método trace_block não existe, loga e retorna nil para continuar
			if strings.Contains(err.Error(), "trace_block") && strings.Contains(err.Error(), "does not exist") {
				log.Printf("trace_block não disponível no nó RPC, ignorando recompensas por trace_block")
				return nil
			}
			return err
		}
		return nil
	}, "trace_block")

	// Se não obteve resultado, retorna mapas vazios
	if len(result) == 0 {
		log.Printf("Nenhum resultado de trace_block obtido ou método indisponível")
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
