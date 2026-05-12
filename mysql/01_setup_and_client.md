# 01 — Setup and Client

> **Goal:** Get MySQL 8 running locally, connect with both the CLI and Workbench, and load the Sakila sample database so every later module has something to query.

---

## 1. Install — analogy + immediate runnable SQL

Think of MySQL like a coffee machine. There's the **server** (the machine itself, always on, holding water and beans = your data), and there's the **client** (the cup you bring to the spout). Most beginners conflate the two. They are separate processes, often on separate machines, talking over TCP/IP on port 3306 (or a Unix socket locally).

### Three install paths (pick one)

**Path A — Docker (recommended for learning):**
```bash
docker run -d \
  --name mysql8 \
  -e MYSQL_ROOT_PASSWORD=rootpw \
  -p 3306:3306 \
  mysql:8.0
```
You're done in 30 seconds. Tear it down with `docker rm -f mysql8` when you're sick of it.

**Path B — Native installer (Windows/macOS):** Download from [dev.mysql.com/downloads/mysql](https://dev.mysql.com/downloads/mysql/). On Windows the MySQL Installer bundles server + Workbench + CLI in one wizard. Pick "Developer Default."

**Path C — Package manager (Linux):**
```bash
# Debian/Ubuntu
sudo apt install mysql-server mysql-client
sudo systemctl start mysql
sudo mysql_secure_installation
```

### First connection
```bash
mysql -u root -p -h 127.0.0.1 -P 3306
```

You're in. Run your first query:
```sql
SELECT VERSION(), NOW(), USER();
```

Expected output:
```
+-----------+---------------------+----------------+
| VERSION() | NOW()               | USER()         |
+-----------+---------------------+----------------+
| 8.0.36    | 2026-05-11 14:22:01 | root@localhost |
+-----------+---------------------+----------------+
```

If you got that, the server, network, auth, and client are all working.

---

## 2. How the client talks to the server — the mechanism

When you type `mysql -u root -p`, here's what actually happens:

1. **Resolve host.** `127.0.0.1` (or `localhost`, which on Linux often means a Unix socket).
2. **TCP handshake** on port 3306.
3. **Server sends a greeting packet** with version, connection ID, and a random salt.
4. **Client sends auth packet** with username and `SHA256(password XOR salt)` — the password never travels in cleartext (since MySQL 8's `caching_sha2_password` plugin).
5. **Session established.** A connection ID is assigned; you're now in your own session with its own variables, transaction state, and current database.
6. **Each query is a packet** sent over the same TCP connection until you disconnect.

You can see your session ID:
```sql
SELECT CONNECTION_ID();
```

And the active connections to the server:
```sql
SHOW PROCESSLIST;
```

This matters because in production you don't open a new TCP+auth handshake per query — that's why connection pools exist (Module 17).

---

## 3. Variations and depth — the three clients you should know

### a. The `mysql` CLI

The classic. Lives forever. Useful flags:
```bash
mysql -u root -p \
  -h db.example.com \
  -P 3306 \
  -D sakila \              # default database
  --prompt="[\u@\h \d]> " \ # custom prompt
  -e "SELECT COUNT(*) FROM film"  # one-shot, exit
```

Inside the prompt, useful built-ins:
```
\h        -- help
\s        -- status (server version, charset, uptime)
\u sakila -- switch database
\T out.txt -- log session output
source script.sql -- run a file
\q        -- quit
```

### b. MySQL Workbench

A GUI from Oracle. Best for: visualizing schemas (it auto-draws ER diagrams from your DB), browsing data, and running EXPLAIN with a visual plan. Worst for: editing huge data sets — it's slow.

Install, connect with `Database → Connect to Database`, host `127.0.0.1`, user `root`. The "Schemas" panel on the left lists databases.

### c. Modern alternatives

- **DBeaver** (free, cross-platform, supports every DB).
- **TablePlus** (paid, gorgeous UI, macOS-first).
- **DataGrip** (JetBrains, paid, best autocomplete).
- **VSCode + SQLTools extension** for in-editor queries.

You'll use the CLI 80% of the time. Pick a GUI for the other 20% (schema visualization, data browsing).

---

## 4. Practical application — load the Sakila sample DB

Sakila is Oracle's official sample DB: a fictional DVD rental store. 16 tables, ~16k rows, well-normalized. We use it in every module.

```bash
# Download
curl -O https://downloads.mysql.com/docs/sakila-db.zip
unzip sakila-db.zip
cd sakila-db

# Load (two scripts: schema, then data)
mysql -u root -p < sakila-schema.sql
mysql -u root -p < sakila-data.sql
```

Verify:
```sql
USE sakila;
SHOW TABLES;
SELECT COUNT(*) FROM film;     -- should return 1000
SELECT COUNT(*) FROM customer; -- should return 599
SELECT COUNT(*) FROM rental;   -- should return 16044
```

Quick exploratory query (your first real one):
```sql
SELECT f.title, c.name AS category, f.rental_rate
FROM film f
JOIN film_category fc ON fc.film_id = f.film_id
JOIN category c       ON c.category_id = fc.category_id
WHERE c.name = 'Comedy'
ORDER BY f.rental_rate DESC
LIMIT 5;
```

If that returns 5 comedy films sorted by rental rate, you have a working learning environment.

---

## 5. Common Mistakes & Gotchas

- **`localhost` ≠ `127.0.0.1` on Linux.** `localhost` triggers a Unix socket connection (ignores `-P`); `127.0.0.1` forces TCP. If you can't connect, try the other.
- **MySQL 8 changed the default auth plugin** to `caching_sha2_password`. Older clients/drivers fail with "Authentication plugin not supported." Fix on the user, not the server: `ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY 'pw';` — but only as a last resort; upgrade your driver.
- **Forgetting `USE dbname;`.** A naked `SELECT * FROM film` in a fresh session errors with "No database selected." Either `USE sakila;` or qualify: `SELECT * FROM sakila.film`.
- **Case sensitivity is platform-dependent.** Table names are case-sensitive on Linux, case-insensitive on Windows/macOS by default. This bites cross-platform teams. Set `lower_case_table_names=1` in `my.cnf` and stick to lowercase.
- **The `mysql` CLI swallows newlines until it sees `;` or `\G`.** If you forget the semicolon, the prompt becomes `->` waiting forever. `\c` cancels the buffered statement.
- **`SELECT * FROM big_table`** in the CLI dumps everything to your screen. Always `LIMIT` while exploring, or use `\P less` to page.
- **vs. PostgreSQL:** MySQL stores databases as folders on disk (`/var/lib/mysql/sakila/`). "Database" and "schema" are synonyms in MySQL but not in Postgres. Don't transfer mental models without checking.

---

## 🎯 Key Takeaways

- **Server and client are separate processes.** Connection pooling, auth plugins, and network latency all flow from this fact.
- **The CLI is your power tool.** GUIs are good for schema visualization, but every senior engineer has the CLI in muscle memory.
- **Always load a real sample DB.** Toy `(id, name)` tables won't teach you joins, indexes, or query plans. Sakila will.
- **MySQL 8 broke auth-plugin compatibility** with older drivers. If a connection fails mysteriously, that's the first thing to check.
- **`localhost` vs `127.0.0.1` matters on Linux.** Burn this into memory before you waste an hour on it.

*← [roadmap](./00_roadmap.md) | [next → Relational Model](./02_relational_model.md)*
