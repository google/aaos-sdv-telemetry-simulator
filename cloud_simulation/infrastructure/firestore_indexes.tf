/*
 *  Copyright 2025 Google LLC
 */

locals {
  firestore_indexes = {
    # Indexes for filtering by status and sorting
    status_received_at_asc = {
      collection = "records"
      fields = [
        { field_path = "status", order = "ASCENDING" },
        { field_path = "received_at", order = "ASCENDING" }
      ]
    },
    status_received_at_desc = {
      collection = "records"
      fields = [
        { field_path = "status", order = "ASCENDING" },
        { field_path = "received_at", order = "DESCENDING" }
      ]
    },
    status_status_updated_at_asc = {
      collection = "records"
      fields = [
        { field_path = "status", order = "ASCENDING" },
        { field_path = "status_updated_at", order = "ASCENDING" }
      ]
    },
    status_status_updated_at_desc = {
      collection = "records"
      fields = [
        { field_path = "status", order = "ASCENDING" },
        { field_path = "status_updated_at", order = "DESCENDING" }
      ]
    },
    status_started_at_asc = {
      collection = "records"
      fields = [
        { field_path = "status", order = "ASCENDING" },
        { field_path = "started_at", order = "ASCENDING" }
      ]
    },
    status_started_at_desc = {
      collection = "records"
      fields = [
        { field_path = "status", order = "ASCENDING" },
        { field_path = "started_at", order = "DESCENDING" }
      ]
    },

    # Indexes for filtering by owner and sorting
    owner_received_at_asc = {
      collection = "records"
      fields = [
        { field_path = "owner", order = "ASCENDING" },
        { field_path = "received_at", order = "ASCENDING" }
      ]
    },
    owner_received_at_desc = {
      collection = "records"
      fields = [
        { field_path = "owner", order = "ASCENDING" },
        { field_path = "received_at", order = "DESCENDING" }
      ]
    },
    owner_status_updated_at_asc = {
      collection = "records"
      fields = [
        { field_path = "owner", order = "ASCENDING" },
        { field_path = "status_updated_at", order = "ASCENDING" }
      ]
    },
    owner_status_updated_at_desc = {
      collection = "records"
      fields = [
        { field_path = "owner", order = "ASCENDING" },
        { field_path = "status_updated_at", order = "DESCENDING" }
      ]
    },
    owner_started_at_asc = {
      collection = "records"
      fields = [
        { field_path = "owner", order = "ASCENDING" },
        { field_path = "started_at", order = "ASCENDING" }
      ]
    },
    owner_started_at_desc = {
      collection = "records"
      fields = [
        { field_path = "owner", order = "ASCENDING" },
        { field_path = "started_at", order = "DESCENDING" }
      ]
    },

    # Indexes for filtering by both status and owner, with different sort orders
    status_owner_received_at_desc = {
      collection = "records"
      fields = [
        { field_path = "status", order = "ASCENDING" },
        { field_path = "owner", order = "ASCENDING" },
        { field_path = "received_at", order = "DESCENDING" }
      ]
    },
    status_owner_status_updated_at_desc = {
      collection = "records"
      fields = [
        { field_path = "status", order = "ASCENDING" },
        { field_path = "owner", order = "ASCENDING" },
        { field_path = "status_updated_at", order = "DESCENDING" }
      ]
    },
    status_owner_started_at_desc = {
      collection = "records"
      fields = [
        { field_path = "status", order = "ASCENDING" },
        { field_path = "owner", order = "ASCENDING" },
        { field_path = "started_at", order = "DESCENDING" }
      ]
    }
  }
}
