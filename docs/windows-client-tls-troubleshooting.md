# Windows client: TLS certificate is not trusted

## Symptoms

The Windows client refuses to log in over HTTPS with:

```text
tls: failed to verify certificate: x509: certificate signed by unknown authority
```

This is **not** an incorrect username/password. The TLS handshake fails
before the Home AI Core login handler can authenticate anything.

The embedded Windows client's transport still requires normal TLS hostname
and certificate-chain verification. Never turn off verification to bypass
the error.

## Verify which certificate the server presents

On the trusted Linux Home AI Core server, inspect the **public certificate**:

```sh
sudo openssl x509 -in /var/lib/home-ai-core/tls/server.crt \
  -noout -subject -issuer -dates -ext subjectAltName -fingerprint -sha256
```

For an administrator-provided TLS certificate, inspect the configured
`HOME_AI_TLS_CERT` file instead. Check the service environment at
`/etc/home-ai-core/home-ai-core.env` and the unit:

```sh
sudo systemctl cat home-ai-core
```

Check separately from the Windows computer that the address and port actually
present that certificate (no password/login required). A certificate read
from a potentially intercepted network is **untrusted evidence** until its
fingerprint is confirmed independently against the server console.

A default self-signed Home AI Core certificate is created under
`/var/lib/home-ai-core/tls/` and includes localhost, the Linux host name
and detected server IP addresses **at creation time**. It does not add
an arbitrary public DNS name such as `homeaicore.ru` automatically.

Consequently, connecting to `https://homeaicore.ru:82` with a default
certificate may fail **both** chain trust and hostname validation.
Importing the default public certificate on the Windows PC **does not**
fix a missing `homeaicore.ru` Subject Alternative Name (SAN).

## Recommended resolution for a public DNS name

Provision a certificate for the *actual connecting hostname*, for example
`homeaicore.ru`, issued by a trusted public CA (ACME/Let's Encrypt).
A certificate is valid for the same domain regardless of the TCP port:
there is no need to move Core off HTTPS port 82.

For servers behind a home router, prefer **ACME DNS-01** with automatic
renewal using your DNS provider's plugin and a narrowly scoped DNS API token.
This avoids requiring public inbound HTTP/HTTPS challenge ports.
The DNS provider and its secure credential setup are administrator-specific.

Once the certificate is issued and the certificate/key pair is backed up,
prepare a read-only service-accessible location (replace the example source
paths with the real issued files):

```sh
sudo install -d -o root -g home-ai-core -m 0750 /etc/home-ai-core/tls
sudo install -o root -g home-ai-core -m 0640 /path/to/fullchain.pem /etc/home-ai-core/tls/fullchain.pem
sudo install -o root -g home-ai-core -m 0640 /path/to/privkey.pem /etc/home-ai-core/tls/privkey.pem
sudo openssl x509 -in /etc/home-ai-core/tls/fullchain.pem \
  -noout -subject -issuer -ext subjectAltName -dates
```

Edit `/etc/home-ai-core/home-ai-core.env` as root; preserve existing
settings, and set **both** variables:

```dotenv
HOME_AI_TLS_CERT=/etc/home-ai-core/tls/fullchain.pem
HOME_AI_TLS_KEY=/etc/home-ai-core/tls/privkey.pem
```

Confirm that the certificate SAN contains the exact server name and the pair
matches; only then restart Core during a maintenance window:

```sh
sudo systemctl restart home-ai-core
sudo systemctl status home-ai-core --no-pager
```

Use a reviewed automatic renewal deployment hook to refresh both installed
files safely and restart Core on successful renewal. **Do not** assume that
certificate-manager files in an inaccessible, root-only live directory will
be readable by the `home-ai-core` service user.

On Windows, verify that HTTPS certificate checks now succeed:

```powershell
curl.exe --fail --show-error --verbose https://homeaicore.ru:82/health
```

If there is a name mismatch, confirm the SAN and the *exact* hostname
entered in the Windows client. If the chain is still untrusted, confirm
that `fullchain.pem` includes the required intermediates and that the
issuing CA is trusted on the client. Do not use `-k` or
`InsecureSkipVerify`.

## Local/private CA alternative

If the address used by the Windows client is already present in SAN,
an independently fingerprint-verified public certificate / CA can be
imported into the Windows user trust store. Alternatively, the client
supports a PEM file through its `HOME_AI_CA_FILE` environment variable:

```powershell
$env:HOME_AI_CA_FILE = 'C:\\Trusted\\verified-home-ai-ca.pem'
& 'C:\\Path\\home-ai-windows-client.exe' settings
```

The environment variable applies only to that process and children.
The background tray agent has its own environment and must be restarted
with that setting if you choose this approach.

**Important:** importing a CA alone does not bypass SAN hostname checks.
For `homeaicore.ru`, the server's presented leaf certificate must still
include `homeaicore.ru` in its SAN.

Never upload `server.key` or other private keys to diagnostics, issue
comments or a certificate trust dialog.
