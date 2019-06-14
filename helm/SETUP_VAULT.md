## Vault Setup

Use Helm to setup Vault with custom values:  
`helm install --name vault -f vault-values.yaml incubator/vault`

Once helm completes the setup, run:
```
export POD_NAME=$(kubectl get pods --namespace default -l "app=vault" -o jsonpath="{.items[0].metadata.name}")
kubectl port-forward --namespace default $POD_NAME 8200:8200
```

In another terminal, run:
```
export VAULT_ADDR=http://localhost:8200

vault status
```

Unseal Vault:

```
vault operator init -key-shares=1 -key-threshold=1
vault operator unseal
vault login
```

Example Vault usage:

```
vault secrets list
vault write secret/foo value=bar
```

## Consul integration

In order to protect the Consul catalog or KV with ACLs, such as in the scenario of storing the terraform remote state on Consul, one of the best solutions is to manage the tokens with Vault.

To do that, enable the Consul secrets engine with the command: `vault secrets enable consul`.

In Consul you have to create a policy for terraform (use the name "terraform"), like the following:

```
key_prefix "terraform/" {
  policy = "read"
}

key_prefix "terraform/" {
  policy = "write"
}
```

Apply the policy in Vault using a valid ACL Token (related to the "global-management" policy of Consul) running:

```
vault write consul/config/access address=consul-consul-server.default.svc.cluster.local:8500 token=ADMIN-TOKEN```

vault write consul/roles/terraform policies=terraform
```

Now it's possible to get ACL Token for the policy "terraform" from Vault, with the command: `vault read consul/creds/terraform` and use it to init your terraform project.

## Terraform

An example project is in the "test/terraform" folder of the repository. Move there and run the init command (be sure to set the CONSUL_HTTP_TOKEN env var with the generated token before!):

```
export CONSUL_HTTP_TOKEN=CHANGE-WITH-THE-GENERATED-TOKEN

terraform init

Initializing the backend...

Successfully configured the backend "consul"! Terraform will automatically
use this backend unless the backend configuration changes.

Terraform has been successfully initialized!
```

As usual, you can use "terraform plan" and "terraform apply" to run the tool and eventually save the state on Consul KV under the "terraform" key. Double-check it in Consul.

## Authentication

Vault supports a number of authentication methods. The simplest one is probably the "userpass" method. 

Run `vault auth enable userpass` to enable the method.

Before creating the first user, you must define a Vault policy in order to authorize the associated users to read the "consul/creds/terraform" path.

The policy (use the name "terraform") could be the following:

```
path "consul/creds/terraform" {
  capabilities = ["read"]
}
```

To create a user and then log in with it, run the following commands:

```
vault write auth/userpass/users/terraform password=tf policies=terraform`

vault login -method=userpass username=terraform password=tf
```