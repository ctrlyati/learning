# 16 — Security: Users, Privileges, Roles, Injection, Encryption

> **Goal:** Lock down MySQL the way a senior backend engineer should — least-privilege users, parameterized queries, encryption in transit and at rest, and the operational discipline that prevents a 3am breach.

---

## 1. Users and privileges — analogy + runnable SQL

A MySQL user is `'name'@'host'`. The host part is half the identity — `'app'@'10.0.0.5'` and `'app'@'%'` are different users.

```sql
-- Create user
CREATE USER 'app'@'10.%.%.%' IDENTIFIED BY 'strongpw';

-- Grant only what's needed
GRANT SELECT, INSERT, UPDATE, DELETE ON sakila.* TO 'app'@'10.%.%.%';

-- Apply
FLUSH PRIVILEGES;  -- usually automatic; sometimes needed after manual mysql.user edits

-- Inspect
SHOW GRANTS FOR 'app'@'10.%.%.%';
```

Never:
- Grant `ALL PRIVILEGES` to an application user.
- Give the application user `WITH GRANT OPTION`.
- Use `'%'` for the host on a production user.
- Store passwords in plaintext config (use a secrets manager).

---

## 2. The privilege model — mechanism

Privileges live at multiple scopes (most-specific wins):
- **Global** (`*.*`) — all databases. Reserved for admins.
- **Database** (`db.*`) — all tables in a DB.
- **Table** (`db.table`) — individual table.
- **Column** (`db.table(col)`) — column-level (rare, useful for sensitive cols).
- **Routine** — procedure/function execute privilege.

Common privileges: `SELECT`, `INSERT`, `UPDATE`, `DELETE`, `CREATE`, `DROP`, `ALTER`, `INDEX`, `EXECUTE`, `CREATE VIEW`, `SHOW VIEW`, `GRANT OPTION`.

Admin-only: `RELOAD`, `SHUTDOWN`, `PROCESS`, `SUPER`, `REPLICATION SLAVE`, `REPLICATION CLIENT`.

### Roles (MySQL 8.0+)

Finally — proper roles instead of GRANT-spaghetti.

```sql
CREATE ROLE 'app_read', 'app_write', 'app_admin';

GRANT SELECT ON sakila.* TO 'app_read';
GRANT INSERT, UPDATE, DELETE ON sakila.* TO 'app_write';
GRANT ALL ON sakila.* TO 'app_admin';

-- Assign to user
CREATE USER 'reader'@'%' IDENTIFIED BY 'pw';
GRANT 'app_read' TO 'reader'@'%';
SET DEFAULT ROLE 'app_read' TO 'reader'@'%';
```

The user must `SET ROLE 'app_read';` per session, OR use `SET DEFAULT ROLE` so it's automatic.

### Authentication plugins

MySQL 8 default: **`caching_sha2_password`** — modern hash, requires TLS or RSA key exchange.

Older clients/drivers may need:
```sql
ALTER USER 'app'@'%' IDENTIFIED WITH mysql_native_password BY 'pw';
```
But upgrade your driver instead — `mysql_native_password` is deprecated.

### Password policy
```sql
INSTALL COMPONENT 'file://component_validate_password';
SHOW VARIABLES LIKE 'validate_password%';
SET GLOBAL validate_password.policy = 'STRONG';
```

Plus:
- `password_history` — can't reuse last N passwords.
- `password_reuse_interval` — can't reuse for N days.
- `default_password_lifetime` — force rotation.

---

## 3. SQL injection, encryption, and hardening — depth

### SQL injection — the #1 web vulnerability

```python
# CATASTROPHIC
sql = "SELECT * FROM user WHERE email = '" + user_input + "'"

# CORRECT — parameterized
cursor.execute("SELECT * FROM user WHERE email = %s", (user_input,))
```

The fix is universal across drivers: **always use placeholders** (`?`, `%s`, `$1` depending on driver). The driver and server keep the parameter values separate from the SQL text — there's no string concat to subvert.

Defenses in depth:
- **Parameterize.** Always.
- **Least-privilege users** — even successful injection on an `app_read` user can't `DROP TABLE`.
- **WAF / input validation** — secondary, not primary.
- **Stored procedures with `DEFINER`** — limit what raw SQL the app needs to execute.

### Encryption in transit (TLS)

```sql
-- Server (my.cnf)
[mysqld]
require_secure_transport = ON
ssl_ca = /etc/mysql/ca.pem
ssl_cert = /etc/mysql/server-cert.pem
ssl_key = /etc/mysql/server-key.pem

-- Per-user enforcement
ALTER USER 'app'@'%' REQUIRE SSL;        -- any TLS
ALTER USER 'app'@'%' REQUIRE X509;       -- client cert
ALTER USER 'app'@'%' REQUIRE SUBJECT '...' ISSUER '...';  -- specific cert
```

Client side:
```bash
mysql --ssl-mode=REQUIRED -u app -p -h db.example.com
```

`ssl-mode` options: `DISABLED`, `PREFERRED`, `REQUIRED`, `VERIFY_CA`, `VERIFY_IDENTITY`. **Production = `VERIFY_IDENTITY`.** Anything less is vulnerable to MITM.

### Encryption at rest

InnoDB tablespace encryption (8.0+):
```sql
INSTALL COMPONENT 'file://component_keyring_file';

CREATE TABLE sensitive (
  id INT PRIMARY KEY,
  ssn VARCHAR(11)
) ENCRYPTION='Y';

-- Or for the whole tablespace:
ALTER INSTANCE ROTATE INNODB MASTER KEY;
```

Cloud providers (RDS, Aurora, Cloud SQL) usually handle this transparently. On-prem: keyring plugin (file, KMIP, AWS KMS).

Beyond tablespace encryption:
- **Column-level encryption** — encrypt sensitive fields in the app before insert. Often the right answer for PII.
- **TDE** (transparent data encryption) protects against disk theft, not against an attacker with DB credentials.

### Auditing

The MySQL Enterprise Audit plugin logs every statement. Free alternatives:
- **MariaDB Audit Plugin** (works with MySQL).
- **General log** — every query (massive volume; for debugging only).
- **Performance schema** events.

### Hardening checklist

- [ ] Run `mysql_secure_installation` after fresh install.
- [ ] No `''@'%'` or `'root'@'%'` users.
- [ ] No anonymous accounts.
- [ ] `bind-address = 127.0.0.1` if only local apps connect; otherwise restrict via firewall/VPC.
- [ ] TLS required for non-local connections.
- [ ] Distinct user per app; least-privilege.
- [ ] Password rotation + complexity policy.
- [ ] At-rest encryption for sensitive tables.
- [ ] Audit logging on or shipped to SIEM.
- [ ] Backups encrypted (gpg, KMS).
- [ ] Patches applied — track CVEs for MySQL.

---

## 4. Practical application — production user setup

```sql
-- Roles
CREATE ROLE 'rd_read', 'rd_write', 'rd_ops', 'rd_audit';

-- Read role: only SELECT
GRANT SELECT ON sakila.* TO 'rd_read';

-- Write role: SELECT + DML, no DDL
GRANT SELECT, INSERT, UPDATE, DELETE ON sakila.* TO 'rd_write';

-- Ops role: + EXECUTE on procs, no DDL
GRANT SELECT, INSERT, UPDATE, DELETE, EXECUTE ON sakila.* TO 'rd_ops';

-- Audit role: read-only on audit tables
GRANT SELECT ON sakila.audit_* TO 'rd_audit';

-- Application user (least privilege for the app)
CREATE USER 'app_sakila'@'10.%.%.%'
  IDENTIFIED BY 'reset-me-immediately'
  REQUIRE SSL
  PASSWORD EXPIRE INTERVAL 90 DAY;
GRANT 'rd_write' TO 'app_sakila'@'10.%.%.%';
SET DEFAULT ROLE 'rd_write' TO 'app_sakila'@'10.%.%.%';

-- Reporting user (read-only, separate analytics network)
CREATE USER 'reporting'@'10.10.%.%'
  IDENTIFIED BY 'reset-me'
  REQUIRE SSL;
GRANT 'rd_read' TO 'reporting'@'10.10.%.%';
SET DEFAULT ROLE 'rd_read' TO 'reporting'@'10.10.%.%';

-- DBA — separate user, individual not shared, MFA at the OS layer
CREATE USER 'dba_yati'@'10.0.0.%'
  IDENTIFIED BY '...'
  REQUIRE X509;
GRANT ALL PRIVILEGES ON *.* TO 'dba_yati'@'10.0.0.%' WITH GRANT OPTION;

-- Lock down root
ALTER USER 'root'@'localhost' PASSWORD EXPIRE;  -- force rotation
-- ...or better: disable root remote login entirely
DROP USER 'root'@'%';
```

### Verifying TLS is in use

```sql
SHOW STATUS LIKE 'Ssl_cipher';
-- empty = no TLS; cipher name = TLS active

SELECT user, host, ssl_type FROM mysql.user;
```

---

## 5. Common Mistakes & Gotchas

- **String-concatenated SQL.** This bug never goes out of style. Train your reviewers.
- **`'%'` host wildcards.** Restrict to subnets at minimum.
- **Sharing a single DB user across apps.** When something goes wrong, you can't tell which app did it; rotating the password breaks everyone.
- **GRANT ALL.** Read-only reports don't need DELETE.
- **No TLS.** Sniffable on any network you don't control.
- **`ssl-mode=REQUIRED` without `VERIFY_IDENTITY`.** Stops eavesdropping but not MITM with a forged cert.
- **Hard-coded passwords in code/config.** Use Vault, AWS Secrets Manager, etc.
- **`mysql_native_password` because the driver complained.** Update the driver.
- **No audit log.** Compliance audits will find you.
- **Backups in plain text on shared storage.** Encrypt backups.
- **Forgotten test users.** `dev`, `test`, `migrator` — periodic audit.
- **vs. Postgres:** Postgres has row-level security policies (RLS) for fine-grained per-row access. MySQL doesn't have native RLS — emulate with views + invoker rights.

---

## 🎯 Key Takeaways

- **Least privilege is the law.** One role per access pattern; one user per app.
- **Parameterize every query.** SQL injection is solved by discipline, not WAFs.
- **TLS with `VERIFY_IDENTITY` everywhere** that crosses a network boundary you don't fully own.
- **Roles in MySQL 8** finally make user management sane. Use them.
- **An audit log + tested restore + patched server** are the three things that determine whether an incident becomes an outage or a breach.

*← [15 backup & replication](./15_backup_replication.md) | [next → Production Operations](./17_production_ops.md)*
