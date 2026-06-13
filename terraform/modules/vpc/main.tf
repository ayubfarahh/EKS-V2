resource "aws_vpc" "main" {
    cidr_block = "10.0.0.0/16"
    enable_dns_support = true
    enable_dns_hostnames = true
  
}

resource "aws_internet_gateway" "igw" {
    vpc_id = aws_vpc.main.id
  
}

resource "aws_eip" "nat_eip" {
  
}

resource "aws_nat_gateway" "ngw" {
  subnet_id = values(aws_subnet.public)[0].id
  allocation_id = aws_eip.nat_eip.id

}

resource "aws_subnet" "public" {
  for_each = {
    for k, v in local.subnets : k => v 
    if v.type == "public" 
  }
  
  vpc_id     = aws_vpc.main.id
  cidr_block = each.value.cidr
  availability_zone = each.value.az
  map_public_ip_on_launch = true 

  tags = {
  "kubernetes.io/role/elb" = "1"
}

}


resource "aws_subnet" "private" {
  for_each = {
    for k, v in local.subnets : k => v
    if v.type == "private"
  }

  vpc_id            = aws_vpc.main.id
  cidr_block        = each.value.cidr
  availability_zone = each.value.az

  tags = {
  "kubernetes.io/role/internal-elb" = "1"
}

}

resource "aws_route_table" "public_rt" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.igw.id
  }
}

resource "aws_route_table" "private_rt" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.ngw.id
  }

}

resource "aws_route_table_association" "pub_asso" {
  for_each = {
    for k, v in local.subnets : k => v if v.type == "public"
  }
  
  subnet_id      = aws_subnet.public[each.key].id
  route_table_id = aws_route_table.public_rt.id
}

resource "aws_route_table_association" "priv_asso" {
  for_each = {
    for k, v in local.subnets : k => v if v.type == "private"
  }
  subnet_id      = aws_subnet.private[each.key].id
  route_table_id = aws_route_table.private_rt.id
}