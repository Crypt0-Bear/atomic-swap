# ETH-XMR Atomic Swaps

This is a WIP implementation of ETH-XMR atomic swaps, currently in the pre-production development phase. It currently consists of `swapd` and `swapcli` binaries, the swap daemon and swap CLI tool respectively, which allow for peers to discover each other over the network, query peers for their current available offers, and the ability to make and take swap offers and perform the swap protocol. The `swapd` program has a JSON-RPC endpoint which the user can use to interact with it. `swapcli` is a command-line utility that interacts with `swapd` by performing RPC calls. 

## Disclaimer

**This code is unaudited and under active development and should not be used on mainnet!** Running this on mainnet may result in loss of funds.

## Protocol

Please see the [protocol documentation](docs/protocol.md) for how it works.

## Instructions

### Requirements

- go 1.17
- ganache-cli (can be installed with `npm i -g ganache-cli`) I suggest using nvm to install npm: https://github.com/nvm-sh/nvm#installing-and-updating

Note: this program has only been tested on Ubuntu 20.04.

#### Set up development environment

Note: the `scripts/install-monero-linux.sh` script will download the monero binaries needed for you. You can also check out the `scripts/run-unit-tests.sh` script for the commands needed to setup the environment.

Start ganache-cli with determinstic keys:
```
ganache-cli -d
```

Start monerod for regtest, this binary is in the monero bin directory:
```bash
cd ./monero-x86_64-linux-gnu
./monerod --regtest --fixed-difficulty=1 --rpc-bind-port 18081 --offline
```

Create a wallet for "Bob", who will own XMR later on:
```
./monero-wallet-cli // you will be prompted to create a wallet. In the next steps, we will go with "Bob", without password. Remember the name and optionally the password for the upcoming steps
```
You do not need to mine blocks, and you can exit the the wallet-cli once Bob's account has been created by typing "exit".

Start monero-wallet-rpc for Bob on port 18083. Make sure `--wallet-dir` corresponds to the directory the wallet from the previous step is in:
```
./monero-wallet-rpc --rpc-bind-port 18083 --password "" --disable-rpc-login --wallet-dir .
```

Open the wallet:
```bash
curl http://localhost:18083/json_rpc -d '{"jsonrpc":"2.0","id":"0","method":"open_wallet","params":{"filename":"Bob","password":""}}' -H 'Content-Type: application/json'

# {
#   "id": "0",
#   "jsonrpc": "2.0",
#   "result": {
#   }
# }
```

Determine the address of `Bob` by looking at `monero-wallet-rpc` logs, in our case 45GcPCB ... uLkV5bTrZRe
```
# 2022-01-20 21:40:06.460	W Loaded wallet keys file, with public address: 45GcPCBQgCG3tYcYqLdj4iQixpDZYw1MGew4PH1rthp9X2YrB2c2dty1r7SwhbCXw1RJMvfy8cW1UXyeESTAuLkV5bTrZRe
```

Then, mine some blocks on the monero test chain by running the following RPC command, replacing the address with the one from Bob's wallet:
```
curl -X POST http://127.0.0.1:18081/json_rpc -d '{"jsonrpc":"2.0","id":"0","method":"generateblocks","params":{"wallet_address":"45GcPCBQgCG3tYcYqLdj4iQixpDZYw1MGew4PH1rthp9X2YrB2c2dty1r7SwhbCXw1RJMvfy8cW1UXyeESTAuLkV5bTrZRe","amount_of_blocks":100}' -H 'Content-Type: application/json'
```

This will deposit some XMR in Bob's account.


Start monero-wallet-rpc for Alice on port 18084 (note that the directory provided to `--wallet-dir` is where Alice's XMR wallet will end up):
```
./monero-wallet-rpc  --rpc-bind-port 18084 --password "" --disable-rpc-login --wallet-dir .
```
#### Build and run

Build binary:
```
make build
```

This creates `swapd` and `swapcli` binaries in the root directory.

To run as Alice, execute in terminal 1:
```
./swapd --dev-alice
```

Alice will print out a libp2p node address, for example `/ip4/127.0.0.1/tcp/9933/p2p/12D3KooWFUEQpGHQ3PtypLvgnWc5XjrqM2zyvdrZXin4vTpQ6QE5`. This will be used for Bob to connect.

To run as Bob and connect to Alice, replace the bootnode in the following line with what Alice logged, and execute in terminal 2:

```
./swapd --dev-bob --wallet-file Bob --bootnodes /ip4/127.0.0.1/tcp/9933/p2p/12D3KooWFUEQpGHQ3PtypLvgnWc5XjrqM2zyvdrZXin4vTpQ6QE5
```

Note: when using the `--dev-alice` and `--dev-bob` flags, Alice's RPC server runs on http://localhost:5001, Bob's runs on http://localhost:5002 by default.

In terminal 3, we will interact with the swap daemon using `swapcli`.

Firstly, we need Bob to make an offer and advertise it, so that Alice can take it:
```bash
./swapcli make --min-amount 0.1 --max-amount 1 --exchange-rate 0.05 --daemon-addr=http://localhost:5002
# Published offer with ID cf4bf01a0775a0d13fa41b14516e4b89034300707a1754e0d99b65f6cb6fffb9
```

Now, we can have Alice begin discovering peers who have offers advertised.
```bash
./swapcli discover --provides XMR --search-time 3
# [[/ip4/127.0.0.1/tcp/9934/p2p/12D3KooWC547RfLcveQi1vBxACjnT6Uv15V11ortDTuxRWuhubGv /ip4/127.0.0.1/tcp/9934/p2p/12D3KooWC547RfLcveQi1vBxACjnT6Uv15V11ortDTuxRWuhubGv]]
```

Query the returned peer as to how much XMR they can provide and their preferred exchange rate (replace `"--multiaddr"` field with one of the addresses returned in the above step):
```bash
./swapcli query --multiaddr /ip4/192.168.0.101/tcp/9934/p2p/12D3KooWC547RfLcveQi1vBxACjnT6Uv15V11ortDTuxRWuhubGv
# Offer ID=cf4bf01a0775a0d13fa41b14516e4b89034300707a1754e0d99b65f6cb6fffb9 Provides=XMR MinimumAmount=0.1 MaximumAmount=1 ExchangeRate=0.05
```

Now, we can tell Alice to initiate the protocol w/ the peer (Bob), the offer (copy the Offer id from above), and a desired amount to swap:
```bash
./swapcli take --multiaddr /ip4/192.168.0.101/tcp/9934/p2p/12D3KooWC547RfLcveQi1vBxACjnT6Uv15V11ortDTuxRWuhubGv --offer-id cf4bf01a0775a0d13fa41b14516e4b89034300707a1754e0d99b65f6cb6fffb9 --provides-amount 0.05
# Initiated swap with ID=0
```

If all goes well, you should see Alice and Bob successfully exchange messages and execute the swap protocol. The result is that Alice now owns the private key to a Monero account (and is the only owner of that key) and Bob has the ETH transferred to him. On Alice's side, a Monero wallet will be generated in the `--wallet-dir` provided in the `monero-wallet-rpc` step for Alice.

To query the information for an ongoing swap, you can run:
```bash
./swapcli get-ongoing-swap
```

To query information for a past swap using its ID, you can run:
```bash
./swapcli get-past-swap --id <id>
```

### Developer instructions

Please see the [developer docs](docs/developing.md).

## Contributions

If you'd like to contribute, feel free to fork the repo and make a pull request. Please make sure the CI is passing - you can run `make build`, `make lint`, and `make test` to make sure the checks pass locally.

## Donations

The work on this project is currently not funded. If you'd like to donate, you can send XMR to the following address: `48WX8KhD8ECgnRBonmDdUecGt8LtQjjesDRzxAjj7tie4CdhtqeBjSLWHhNKMc52kWayq365czkN3MV62abQobTcT1Xe6xC`