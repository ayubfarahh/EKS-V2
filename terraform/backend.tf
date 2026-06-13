terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.34.0"
    }
  }

  backend "s3" {
    bucket = "eksv2-bucket"
    key    = "terraform.tfstate"
    region = "eu-west-2"
    #use_lockfile = true
  }
}

provider "aws" {

}

