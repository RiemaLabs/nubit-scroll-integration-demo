package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/RiemaLabs/nubit-validator/auxiliary/encoding"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/spf13/cobra"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

const BitcoinConfirmation = 6

func FetchHeight(host string) (uint64, error) {
	param := map[string]interface{}{
		"method": "getblockcount",
		"params": []interface{}{},
	}
	jsonParam, _ := json.Marshal(param)
	reader := strings.NewReader(string(jsonParam))
	res, err := http.Post(host, "application/json", reader)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	// 读取response的body
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return 0, err
	}
	a := map[string]interface{}{}
	json.Unmarshal(body, &a)
	return uint64(a["result"].(float64)), nil
}

func ExtendTxsWithBtcRef(btcHeight uint64, txs [][]byte) ([][]byte, error) {
	tx, err := makeBtcTx(btcHeight)
	if err != nil {
		return nil, err
	}
	var codedHeightExt [][]byte
	codedHeightExt = [][]byte{tx}
	codedHeightExt = append(codedHeightExt, txs...)
	return codedHeightExt, nil
}

func CheckTxsWithBtcRef(btcHeight uint64, latestBtcHeight uint64, latestBtcHeightNotRecorded bool, rpc string) (bool, error) {
	if !latestBtcHeightNotRecorded && btcHeight < latestBtcHeight {
		return false, errors.New("claimed height is less than latest btc height")
	} else if currentBtc, err := FetchHeight(rpc); err != nil {
		return false, err
	} else if math.Abs(float64(int64(currentBtc)-int64(btcHeight))) > BitcoinConfirmation {
		return false, errors.New("btc height is out of range")
	}
	return true, nil
}

func BtcHeightCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "btcHeight [height]",
		Short: "Get btc height by nubit height",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			var height int64
			if len(args) == 0 {
				return errors.New("the parameter cannot be null")
			}
			if h, err := strconv.Atoi(args[0]); err != nil {
				return err
			} else if h > 0 {
				tmp := int64(h)
				height = tmp
			}

			if height == 0 {
				return errors.New("height can't be 0")
			}
			output, err := getBlock(clientCtx, &height)
			if err != nil {
				return errors.New("height too high")
			}
			if output.Block == nil {
				return errors.New("nil block")
			}
			if len(output.Block.Txs) == 0 {
				return errors.New("tx.len == 0")
			}

			BtcHeight, err := GetBtcHeightFromTx(output.Block.Txs[0])
			if err != nil {
				return err
			}
			fmt.Println(BtcHeight)
			return nil
		},
	}

	cmd.Flags().StringP(flags.FlagNode, "n", "tcp://localhost:26657", "Node to connect to")
	return cmd
}

func FindHeightRangeAtBtcHeightCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "FindHeightRange [height]",
		Short: "Get height list by btc height",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			var BtcHeight int64
			if len(args) == 0 {
				return errors.New("the parameter cannot be null")
			}

			if h, err := strconv.Atoi(args[0]); err != nil {
				return err
			} else if h > 0 {
				tmp := int64(h)
				BtcHeight = tmp
			}
			heightList, err := findHeightsByBtcHeight(clientCtx, uint64(BtcHeight))
			if err != nil {
				return err
			}
			fmt.Println(heightList)
			return nil
		},
	}

	cmd.Flags().StringP(flags.FlagNode, "n", "tcp://localhost:26657", "Node to connect to")

	return cmd
}

func getBlock(clientCtx client.Context, height *int64) (*ctypes.ResultBlock, error) {
	if node, err := clientCtx.GetNode(); err != nil {
		return nil, err
	} else {
		return node.Block(context.Background(), height)
	}
}

func getLatestHeight(clientCtx client.Context) (uint64, error) {
	node, err := clientCtx.GetNode()
	if err != nil {
		return 0, err
	}
	status, err := node.Status(context.Background())
	if err != nil {
		return 0, err
	}
	return uint64(status.SyncInfo.LatestBlockHeight), nil
}

func findHeightsByBtcHeight(clientCtx client.Context, btcHeight uint64) ([]int64, error) {
	if lastHeight, err := getLatestHeight(clientCtx); err != nil {
		return nil, err
	} else {
		return findRange(clientCtx, lastHeight, float64(btcHeight)), nil
	}
}

func findRange(clientCtx client.Context, maxIndex uint64, height float64) []int64 {
	left := binarySearch(clientCtx, maxIndex, height-0.5)
	right := binarySearch(clientCtx, maxIndex, height+0.5)
	if left >= right || left == -1 || right == -1 {
		return []int64{}
	}
	var r []int64
	for i := left; i < right; i++ {
		r = append(r, i)
	}
	return r
}

func binarySearch(clientCtx client.Context, maxIndex uint64, target float64) int64 {
	left, right := uint64(1), maxIndex
	for left < right {
		mid := int64(left + (right-left)/2)
		output, _ := getBlock(clientCtx, &mid)
		if output == nil || output.Block == nil {
			return -1
		}
		height, _ := GetBtcHeightFromTx(output.Block.Txs[0])
		if float64(height) < target {
			left = uint64(mid) + 1
		} else {
			right = uint64(mid)
		}
	}
	return int64(left)
}

func makeBtcTx(btcHeight uint64) ([]byte, error) {
	// make TxConfig
	txCfg := makeTxConfig()
	tx := txCfg.NewTxBuilder()
	btcHeightStr := strconv.FormatUint(btcHeight, 10)
	tx.SetMemo(btcHeightStr)

	return txCfg.TxEncoder()(tx.GetTx())
}

func GetBtcHeightFromTx(txByte []byte) (uint64, error) {
	txCfg := makeTxConfig()
	decodedTx, err := txCfg.TxDecoder()(txByte)
	if err != nil {
		return 0, err
	}
	if txWithMemo, ok := decodedTx.(types.TxWithMemo); ok {
		return strconv.ParseUint(txWithMemo.GetMemo(), 10, 64)
	}
	return 0, errors.New("btcTx does not have memo")
}

func makeTxConfig() client.TxConfig {
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	marshaler := codec.NewProtoCodec(interfaceRegistry)
	txCfg := tx.NewTxConfig(marshaler, tx.DefaultSignModes)
	dec := txCfg.TxDecoder()
	dec = encoding.IndexWrapperDecoder(dec)
	txCfg.SetTxDecoder(dec)
	return txCfg
}
