resource "aws_eks_addon" "pod_identity_agent" {
  cluster_name                = var.eks_cluster_name
  addon_name                  = "eks-pod-identity-agent"
  
}

module "external_dns_pod_identity" {
  source = "terraform-aws-modules/eks-pod-identity/aws"

  name = "external-dns"

  attach_external_dns_policy    = true
  external_dns_hosted_zone_arns = ["arn:aws:route53:::hostedzone/Z0705516CKV60PQH3XUN"]

  associations = {
    this = {
      cluster_name    = var.eks_cluster_name
      namespace       = "external-dns"
      service_account = "external-dns"
    }
  }

}


module "cert_manager_pod_identity" {
  source = "terraform-aws-modules/eks-pod-identity/aws"

  name = "cert-manager"

  attach_cert_manager_policy    = true
  cert_manager_hosted_zone_arns = ["arn:aws:route53:::hostedzone/Z0705516CKV60PQH3XUN"]

  associations = {
    this = {
      cluster_name    = var.eks_cluster_name
      namespace       = "cert-manager"
      service_account = "cert-manager"
    }
  }
}

module "worker_pod_identity" {
  source = "terraform-aws-modules/eks-pod-identity/aws"

  name = "worker"

  attach_custom_policy = true
  policy_statements = [
    {
      sid     = "SQSConsume"
      actions = [
        "sqs:ReceiveMessage",
        "sqs:DeleteMessage",
        "sqs:GetQueueAttributes",
      ]
      resources = [var.order_events_queue_arn]
    }
  ]

  associations = {
    this = {
      cluster_name    = var.eks_cluster_name
      namespace       = "app"
      service_account = "worker"
    }
  }
}


module "publisher_pod_identity" {
  source = "terraform-aws-modules/eks-pod-identity/aws"

  name = "publisher"

  attach_custom_policy = true
  policy_statements = [
    {
      sid     = "SQSPublish"
      actions = [
        "sqs:SendMessage"
      ]
      resources = [var.order_events_queue_arn]
    }
  ]

  associations = {
    this = {
      cluster_name    = var.eks_cluster_name
      namespace       = "app"
      service_account = "publisher"
    }
  }
}
// Varialise hosted zone later 