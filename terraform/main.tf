module "vpc" {
    source = "./modules/vpc"
  
}

module "eks" {
    source = "./modules/eks"
    private_subnet_ids = module.vpc.private_subnet_ids
    vpc_id = module.vpc.vpc_id
    public_subnet_ids = module.vpc.public_subnet_ids
}