module "vpc" {
    source = "./modules/vpc"
  
}

module "eks" {
    source = "./modules/eks"
    private_subnet_ids = module.vpc.private_subnet_ids
    vpc_id = module.vpc.vpc_id
    public_subnet_ids = module.vpc.public_subnet_ids
}

module "pod-id" {
    source = "./modules/pod-id"
    eks_cluster_name = module.eks.eks_cluster_name
    order_events_queue_arn = module.sqs.queue_arn
}

module "sqs" {
    source = "./modules/sqs"
}