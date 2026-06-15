resource "aws_eks_cluster" "eks_cluster" {
  name = "eks-v2-cluster"
  role_arn = aws_iam_role.cluster.arn
  version  = "1.33"

  vpc_config {
    subnet_ids = var.private_subnet_ids
    security_group_ids =  [aws_security_group.cluster_sg.id]
    endpoint_public_access = true
  }

  depends_on = [
    aws_iam_role_policy_attachment.cluster_AmazonEKSClusterPolicy,
  ]
}

resource "aws_iam_role" "cluster" {
  name = "eks-cluster-example"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "sts:AssumeRole",
          "sts:TagSession"
        ]
        Effect = "Allow"
        Principal = {
          Service = "eks.amazonaws.com"
        }
      },
    ]
  })
}

resource "aws_iam_role_policy_attachment" "cluster_AmazonEKSClusterPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
  role       = aws_iam_role.cluster.name
}

resource "aws_iam_role_policy_attachment" "cluster_AmazonEKSVPCResourceController" {
  role       = aws_iam_role.cluster.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSVPCResourceController"

}

resource "aws_eks_node_group" "workers" {
  cluster_name    = aws_eks_cluster.eks_cluster.name
  node_group_name = "workers"
  node_role_arn   = aws_iam_role.worker_permissions.arn
  instance_types = ["t3.medium"]
  subnet_ids      = var.private_subnet_ids
  
  scaling_config {
    desired_size = 2
    max_size     = 3
    min_size     = 2
  }

  update_config {
    max_unavailable = 1
  }

  depends_on = [
    aws_iam_role_policy_attachment.worker_permissions-AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.worker_permissions-AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.worker_permissions-AmazonEC2ContainerRegistryReadOnly,
  ]
}

resource "aws_iam_role" "worker_permissions" {
  name = "eks-node-group-worker"
  
  assume_role_policy = jsonencode({
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
    }]
    Version = "2012-10-17"
  })
}

resource "aws_iam_role_policy_attachment" "worker_permissions-AmazonEKSWorkerNodePolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
  role       = aws_iam_role.worker_permissions.name
}

resource "aws_iam_role_policy_attachment" "worker_permissions-AmazonEKS_CNI_Policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
  role       = aws_iam_role.worker_permissions.name
}

resource "aws_iam_role_policy_attachment" "worker_permissions-AmazonEC2ContainerRegistryReadOnly" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  role       = aws_iam_role.worker_permissions.name
}

resource "aws_security_group" "cluster_sg" {
  vpc_id = var.vpc_id
  name = "cluster-sg"
}

resource "aws_security_group" "worker_sg" {
  vpc_id = var.vpc_id
  name = "worker-sg"

  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    self        = true
  }
  
  ingress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    security_groups = [aws_security_group.cluster_sg.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group_rule" "cluster_to_worker" {
  type              = "ingress"
  from_port         = 10250
  to_port           = 10250
  protocol          = "tcp"
  security_group_id = aws_security_group.worker_sg.id
  source_security_group_id = aws_security_group.cluster_sg.id
}

