terraform {
  backend "consul" {
    lock    = false
    address = "consul.automium.local:8081"
    scheme  = "http"
    path    = "terraform/tfstate"
  }
}