variable "prefix" {
  type = string
}

variable "image_name" {
  type = string
}

variable "flavor_name" {
  type = string
}

variable "root_disk_size" {
  type = string
}

variable "tags" {
  type = map(string)
}

variable "additional_tags" {
  type = map(string)
}

variable "keypair_ssh_name" {
  type = string
}

variable "floating_ip_network" {
  type = string
  default = ""
}

variable "network_port_ids" {
  type = list(string)
}

variable "config_drive" {
  type = bool
  default = false
}

variable "node_index" {
  type = string
}

variable "cloud_config" {
  type = string
}

variable "volume_type" {
  type = string
}

variable "zone" {
  type = string
}
