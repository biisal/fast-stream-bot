# Fast Stream Bot

Fast Stream Bot is a high-performance Telegram bot that lets you generate streamable and downloadable links for any file you send to it. It features a credit system, referral program, and admin tools, making it perfect for managing file sharing.

## Quick Start

The easiest way to install Fast Stream Bot is using our installer script. Run the following command in your terminal:

```bash
curl -sfL https://raw.githubusercontent.com/biisal/fast-stream-bot/main/install.sh | bash
```

This will automatically download the latest version for your system and install it to `~/.fast-stream-bot/bin`.

## Configuration

Before running the bot, you need to configure it.

1.  **Create a folder for your bot:**
    ```bash
    mkdir my-bot
    cd my-bot
    ```

2.  **Create a `.env` file:**
    This file holds your secret keys. Create a file named `.env` and paste the following (fill in your values):

    ```env
    BOT_TOKENS=your_bot_token_from_botfather
    APP_KEY=your_telegram_app_id
    APP_HASH=your_telegram_app_hash
    ADMIN_ID=your_telegram_user_id
    HTTP_PORT=8000
    FQDN=http://your-server-ip:8000
    LOG_CHANNEL_ID=-100xxxxxxxxxx
    DB_CHANNEL_ID=-100xxxxxxxxxx
    MAIN_CHANNEL_ID=-100xxxxxxxxxx
    MAIN_CHANNEL_USERNAME=your_channel_username
    DBSTRING=your-psql-connection-string (get it from neon.com db [one day we will sponsor .. lol])
    REDIS_DBSTRING=your-redis-connection-string (get it from upstash.com)
    ```
    > **Note:** You can get `APP_KEY` and `APP_HASH` from [my.telegram.org](https://my.telegram.org).

3.  **Create a `config.toml` file:**
    This file holds application settings. Create a file named `config.toml`:

    ```toml
    app_name = "Fast Stream Bot"
    env_file = ".env"
    header_image = "https://some-image_url-or-path.com"

    # Credit System Configuration
    max_credits = 500
    min_credits_required = 5
    initial_credits = 50
    increment_credits = 10
    decrement_credits = 10
    ```

## Running the Bot

Once installed and configured, simply run:

```bash
fast-stream-bot
```

You should see the startup logo and a message indicating the bot is valid and running.

## Features at a Glance

-   **Instant Links:** Stream or download files instantly.
-   **Credit System:** Control usage with a built-in credit system.
-   **Channel Lock:** Force users to join a channel to use the bot.
-   **Admin Dashboard:** Ban/unban users and broadcast messages directly from the bot.

---

## For Developers

If you want to contribute or build from source, follow these steps.

### Prerequisites

-   [Go](https://go.dev/dl/) (v1.24+)
-   [Redis](https://redis.io/)
-   [Node.js](https://nodejs.org/) (for Tailwind CSS)
-   [Make](https://www.gnu.org/software/make/)

### Build from Source

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/biisal/fast-stream-bot.git
    cd fast-stream-bot
    ```

2.  **Run in development mode:**
    ```bash
    make dev
    ```

3.  **Build production binary:**
    ```bash
    make build
    ```

## License

[MIT](LICENSE)
