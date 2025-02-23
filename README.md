# Gasflow Server

Gasflow Server is a blockchain transaction monitoring and mempool data collection engine for Ethereum. It streams transaction data, processes mempool statistics, and provides a WebSocket and API interface.

## Features
- Collects Ethereum mempool transactions in real-time.
- Provides a WebSocket server for live transaction streaming.
- Offers an internal API for querying mempool and block data.
- Saves processed block data for further analysis.

## Installation
### Prerequisites
- Go 1.20+
- An Ethereum node (e.g., Geth, Erigon) with WebSocket enabled.

### Clone the repository:
```sh
git clone https://github.com/duoxehyon/gasflow-server.git
cd gasflow-server
```

### Build:
```sh
go build -o gasflow-server
```

### Run:
```sh
./gasflow-server -eth ws://localhost:8546 -enable-ws -enable-api -enable-saving
```

## Configuration
The server can be configured via command-line flags:

| Flag             | Description                                    | Default                     |
|-----------------|--------------------------------|-----------------------------|
| `-eth`          | Ethereum node WebSocket URL            | `ws://localhost:8546`       |
| `-ws-port`      | WebSocket server port                 | `:8080`                     |
| `-api-port`     | Internal API server port             | `:8081`                     |
| `-enable-ws`    | Enable WebSocket server            | `false`                     |
| `-enable-api`   | Enable internal API server        | `false`                     |
| `-enable-saving`| Enable block data saving        | `false`                     |
| `-output`       | Directory to store block data    | `data/`                     |

Example:
```sh
./gasflow-server -eth ws://your-ethereum-node:8546 -ws-port :9000 -api-port :9001 -enable-ws -enable-api -enable-saving -output storage/
```

## API Endpoints
The internal API is only accessible from `localhost`.

### **GET /internal/network**
Returns mempool and network statistics.

**Example Response:**
```json
{
  "count": 12000,
  "p10": 20.3,
  "p30": 22.5,
  "p50": 30.1,
  "p70": 40.2,
  "p90": 60.7,
  "next_block": 1928391,
  "next_base_fee": 25.0
}
```

### **GET /internal/block?block={block_number}**
Retrieves data for a specific block.

**Example Request:**
```sh
curl "http://localhost:8081/internal/block?block=1928390"
```

**Example Response:**
```json
{
  "block": {
    "number": 1928390,
    "timestamp": 1700000000,
    "base_fee": "20.0",
    "gas_used": 15000000
  },
  "network": {
    "mempool": { ... },
    "history": { ... }
  },
  "txs": [
    {
      "hash": "0xabc...",
      "max_fee": "30.5",
      "gas": 21000
      ...
    }
  ]
}
```

## WebSocket API
The WebSocket server provides real-time updates.

### **Connect:**
```
ws://localhost:8080/ws
```

### **Messages Sent:**
WebSocket clients receive mempool updates in JSON format.

**Example Response:**
```json
{
  "count": 12345,
  "fee_histogram": { "10.0": 200, "15.0": 500 },
  "next_base_fee": 25.3
}
```

## Data Storage
If `-enable-saving` is enabled, block data is saved as JSON files in the specified `-output` directory.

- Example file: `storage/block_1928390.json`

## Development
### Run Tests:
```sh
go test ./...
```

### Code Formatting:
```sh
gofmt -s -w .
```

Note: This is an initial version and may have areas for improvement. Contributions and feedback are welcome.

## License
MIT License.
