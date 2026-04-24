# InvestAssist Go Backend

This is the Go-based backend for InvestAssist, an investment tool for tracking and analyzing asset growth using DCA (Dollar Cost Averaging) and VA (Value Averaging) strategies.

## Architecture

The project follows a layered architecture:

- `cmd/server/main.go`: Application entry point.
- `internal/api/`: HTTP handlers and routing using the Echo framework.
- `internal/service/`: Core business logic and strategy calculations.
- `internal/model/`: Domain models and database schema definitions.
- `internal/db/`: Database connection and migrations using GORM.
- `pkg/worker/`: Background jobs (e.g., price history updates).

## Prerequisites

- [Go](https://golang.org/doc/install) (1.16+)
- [PostgreSQL](https://www.postgresql.org/download/)

## Getting Started

1.  **Clone the repository and navigate to the server folder:**
    ```bash
    cd server-go
    ```

2.  **Install dependencies:**
    ```bash
    go mod tidy
    ```

3.  **Set up the database:**
    Create a PostgreSQL database named `invest_assist`.

4.  **Run the server:**
    Set the `DATABASE_URL` environment variable and run the application:
    ```bash
    DATABASE_URL="host=localhost user=postgres password=postgres dbname=invest_assist port=5432 sslmode=disable" go run cmd/server/main.go
    ```
    The server will start on `http://localhost:3000`.

## API Endpoints

- `GET /strategy`: List all investment strategies.
- `POST /strategy`: Create a new investment strategy.
- `GET /history/:id`: Get historical value and growth for a specific strategy.
- `GET /summary/:id`: Get a summary of performance and next actions for a strategy.
- `POST /transaction`: Record a new buy/sell transaction.
- `GET /securities`: List supported securities (e.g., BTC, ETH).

## Environment Variables

- `DATABASE_URL`: The PostgreSQL connection string. Default: `host=localhost user=postgres password=postgres dbname=invest_assist port=5432 sslmode=disable`.
