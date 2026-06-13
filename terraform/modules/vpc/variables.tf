locals {
  subnets = {
    public_1 = {
        cidr = "10.0.1.0/24"
        az = "eu-west-2a"
        type = "public"

    }
    public_2 = {
        cidr = "10.0.2.0/24"
        az = "eu-west-2b"
        type = "public"
    }

    private_1 = {
        cidr = "10.0.3.0/24"
        az = "eu-west-2a"
        type = "private"
    }
    private_2 = {
        cidr = "10.0.4.0/24"
        az = "eu-west-2b"
        type = "private"
    }
  }
}
