package node

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cybrarymin/btcblockchain/chain"
	"github.com/cybrarymin/btcblockchain/helpers"
	"github.com/cybrarymin/btcblockchain/protogen/pb"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type GRPCMsgRelay[Msg any] func(ctx context.Context, conn *grpc.ClientConn, chRelay chan Msg) error

var GRPCTxRelay GRPCMsgRelay[*chain.SignedTransaction] = func(ctx context.Context, conn *grpc.ClientConn, chRelay chan *chain.SignedTransaction) error {
	cln := pb.NewTransactionServiceClient(conn)
	stream, err := cln.ReceiveTransaction(context.Background())
	if err != nil {
		return err
	}
	defer stream.CloseAndRecv()
	for {
		select {
		case <-ctx.Done():
			return nil
		case tx, open := <-chRelay:
			if !open {
				return nil
			}
			jtx, err := helpers.JsonMarshaller(ctx, tx)
			if err != nil {
				continue
			}
			req := &pb.TxReceiveReq{SignedTransaction: jtx}
			err = stream.Send(req)
			if err != nil {
				fmt.Println(err)
				continue
			}
		}
	}
}

var GRPCBlockRelay GRPCMsgRelay[*chain.SignedBlock] = func(ctx context.Context, conn *grpc.ClientConn, chRelay chan *chain.SignedBlock) error {
	cln := pb.NewBlockServiceClient(conn)
	stream, err := cln.ReceiveBlock(ctx)
	if err != nil {
		return err
	}
	defer stream.CloseAndRecv()
	for {
		select {
		case <-ctx.Done():
			return nil
		case blk, open := <-chRelay:
			if !open {
				return nil
			}
			jblk, err := json.Marshal(blk)
			if err != nil {
				fmt.Println(err)
				continue
			}
			req := &pb.BlockReceiveReq{Block: jblk}
			err = stream.Send(req)
			if err != nil {
				return err
			}
		}
	}
}

// msg relayer is gonna used to relay transaction and blocks
type MsgRelay[Msg any, Relay GRPCMsgRelay[Msg]] struct {
	logger                  *zerolog.Logger
	ctx                     context.Context
	wg                      *sync.WaitGroup
	chMsg                   chan Msg
	grpcRelay               Relay
	selfRelay               bool
	peerReader              PeerReader
	wgRelays                *sync.WaitGroup
	chPeerAdd, chPeerRemove chan string
}

func NewMsgRelay[Msg any, Relay GRPCMsgRelay[Msg]](
	ctx context.Context, logger *zerolog.Logger, wg *sync.WaitGroup, cap int,
	grpcRelay Relay, selfRelay bool, peerReader PeerReader,
) *MsgRelay[Msg, Relay] {
	return &MsgRelay[Msg, Relay]{
		logger: logger,
		ctx:    ctx, wg: wg, chMsg: make(chan Msg, cap),
		grpcRelay: grpcRelay, selfRelay: selfRelay, peerReader: peerReader,
		wgRelays:  new(sync.WaitGroup),
		chPeerAdd: make(chan string), chPeerRemove: make(chan string),
	}
}

type TxRelayer interface {
	RelayTx(ctx context.Context, tx *chain.SignedTransaction)
}

type BlockRelayer interface {
	RelayBlock(ctx context.Context, blk *chain.SignedBlock)
}

/*
relay tx is gonna relay the created transaction to the network.
When a user submits a transaction to one node, other nodes in the network need to know about it.
All the nodes should have the same transaction in their memory pool in pending state.
These relayed transactions are gonna be used by block proposer when it is preparing a new block to be validated.
*/
func (r *MsgRelay[Msg, Relay]) RelayTx(ctx context.Context, tx Msg) error {
	r.chMsg <- tx
	return nil
}

/*
Block relaying is the process of propagating newly created blocks across the network so all nodes can validate and add them to their local blockchain.
The authority node (bootstrap node) creates a new block through the BlockProposer
The block is immediately relayed to all peer nodes
Receiving nodes validate and apply the block to their state
*/
func (r *MsgRelay[Msg, Relay]) RelayBlock(ctx context.Context, blk Msg) {
	r.chMsg <- blk
}

/*
RelayMsgs is gonna relay the messages to the peers. the messages can be tx or block.
*/
func (r *MsgRelay[Msg, Relay]) RelayMsgs(period time.Duration) {
	ctx, span := otel.Tracer("RelayMsgs.Tracer").Start(r.ctx, "RelayMsg.Span")
	defer span.End()
	defer r.wg.Done()
	r.wgRelays.Add(1)
	go r.addPeers(period)
	chRelays := make(map[string]chan Msg)
	closeRelays := func() {
		for _, chRelay := range chRelays {
			close(chRelay)
		}
	}
	for {
		select {
		case <-ctx.Done():
			closeRelays()
			r.wgRelays.Wait()
			return
		case peer := <-r.chPeerAdd: // anytime a peer is added to the relayer peer list by addPeers function
			_, exist := chRelays[peer] // check if the peer has the relay channel
			if exist {
				continue
			}
			if r.selfRelay {
				r.logger.Info().Str("peer", peer).Msgf("adding peer as Blk relay endpoint: %v", peer)
			} else {
				r.logger.Info().Str("peer", peer).Msgf("adding peer as Tx relay endpoint: %v", peer)
			}
			chRelay := r.peerRelay(peer)
			chRelays[peer] = chRelay
		case peer := <-r.chPeerRemove:
			_, exist := chRelays[peer]
			if !exist {
				continue
			}
			chRelay := chRelays[peer]
			close(chRelay)
			r.logger.Info().Msgf("removing peer from Tx relay endpoints: %v", peer)
			delete(chRelays, peer)
		case msg := <-r.chMsg:
			for _, chRelay := range chRelays {
				chRelay <- msg
			}
		}
	}
}

/*
addPeers is gonna run periodically in the background to fetch the active peers and add them to the relayer peer list.
If any peer get's deactivated and removed from the actual peer list then relayer will remove it from it's relay peer list as well.
*/
func (r *MsgRelay[Msg, Relay]) addPeers(period time.Duration) {
	defer r.wgRelays.Done()
	tick := time.NewTicker(period)
	defer tick.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-tick.C:
			var peers []string
			if r.selfRelay {
				peers = r.peerReader.SelfPeers()
			} else {
				peers = r.peerReader.Peers()
			}
			for _, peer := range peers {
				r.chPeerAdd <- peer
			}
		}
	}
}

func (r *MsgRelay[Msg, Relay]) peerRelay(peer string) chan Msg {
	chRelay := make(chan Msg)
	r.wgRelays.Add(1)
	go func() {
		defer r.wgRelays.Done()
		conn, err := grpc.NewClient(
			peer, grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			r.logger.Error().Err(err).
				Interface("peer", peer).
				Msg("failed to connect to peer for msg relay")
			r.chPeerRemove <- peer
			return
		}
		defer conn.Close()
		err = r.grpcRelay(r.ctx, conn, chRelay)
		if err != nil {
			r.logger.Error().Err(err).
				Interface("peer", peer).
				Msg("failed to connect to peer for msg relay")
			r.chPeerRemove <- peer
			return
		}
	}()
	return chRelay
}
