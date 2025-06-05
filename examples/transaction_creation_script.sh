#!/bin/bash
for i in `seq 1 16`;
  do
  SIGNED_TX=$(grpcurl -plaintext -d "`cat signtransaction_req.json`" localhost:6881 chain.TransactionService.SignTransaction | jq .transaction )
  grpcurl -plaintext -d "{ \"SignedTransaction\": $SIGNED_TX }" localhost:6881 chain.TransactionService.SendTransaction  
  done;
