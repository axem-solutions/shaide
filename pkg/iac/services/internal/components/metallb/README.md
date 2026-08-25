## TLS

| File | Type | Role |
|---|---|---|
| `server.key` | Private key | Secret key for the server. Never leaves the server. |
| `server.crt` | Server certificate | Contains the server's public key, signed by the CA. Presented to clients during TLS handshake. |
| `ca.crt` | CA certificate | The trust anchor. Contains the CA's public key. Used to verify that `server.crt` is legitimate. |
| `server.pem` | Full chain | `server.crt` + `ca.crt` concatenated. Built locally before deployment — not stored in the repository. |

### How TLS Works With These Files

1. Client connects to the server over HTTPS.
2. Server presents `server.crt`.
3. Client checks: was this cert signed by a CA I trust?
4. If `ca.crt` is in the client's trust store — yes, trusted. Connection proceeds.
5. If not — browser shows "certificate not trusted" warning.

### Where Each File Goes

- **Kubernetes Secret (`tls.crt`)** — `server.crt` + `ca.crt` concatenated (full chain). The Gateway needs the full chain so intermediate certs are delivered to clients.
- **Kubernetes Secret (`tls.key`)** — `server.key`
- **Chrome / OS trust store** — `ca.crt` only. Adding the CA cert tells Chrome to trust all certificates it has signed, including `server.crt`.

### Deployment

For Kubernetes TLS the `tls.crt` field must be the full chain (server cert + CA cert concatenated). Build it first:

```bash
cat server.crt ca.crt > server.pem
```

Add to playbook:

```bash
pulumi config set --stack <env> --secret -- tlsCert "$(cat server.pem)"
pulumi config set --stack <env> --secret -- tlsKey "$(cat server.key)"
```
