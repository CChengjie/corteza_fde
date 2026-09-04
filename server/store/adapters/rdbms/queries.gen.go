package rdbms

// This file is auto-generated.
//
// Changes to this file may cause incorrect behavior and will be lost if
// the code is regenerated.
//

import (
	automationType "github.com/cortezaproject/corteza/server/automation/types"
	composeType "github.com/cortezaproject/corteza/server/compose/types"
	discoveryType "github.com/cortezaproject/corteza/server/discovery/types"
	federationType "github.com/cortezaproject/corteza/server/federation/types"
	actionlogType "github.com/cortezaproject/corteza/server/pkg/actionlog"
	flagType "github.com/cortezaproject/corteza/server/pkg/flag/types"
	labelsType "github.com/cortezaproject/corteza/server/pkg/label/types"
	rbacType "github.com/cortezaproject/corteza/server/pkg/rbac"
	systemType "github.com/cortezaproject/corteza/server/system/types"
	"github.com/doug-martin/goqu/v9"
)

var (
	// actionlogTable represents actionlogs store table
	//
	// This value is auto-generated
	actionlogTable = goqu.T("actionlog")

	// actionlogSelectQuery assembles select query for fetching actionlogs
	//
	// This function is auto-generated
	actionlogSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"ts",
			"actor_ip_addr",
			"actor_id",
			"request_origin",
			"request_id",
			"resource",
			"action",
			"error",
			"severity",
			"description",
			"meta",
		).From(actionlogTable)
	}

	// actionlogInsertQuery assembles query inserting actionlogs
	//
	// This function is auto-generated
	actionlogInsertQuery = func(d goqu.DialectWrapper, res *actionlogType.Action) *goqu.InsertDataset {
		return d.Insert(actionlogTable).
			Rows(goqu.Record{
				"id":             res.ID,
				"ts":             res.Timestamp,
				"actor_ip_addr":  res.ActorIPAddr,
				"actor_id":       res.ActorID,
				"request_origin": res.RequestOrigin,
				"request_id":     res.RequestID,
				"resource":       res.Resource,
				"action":         res.Action,
				"error":          res.Error,
				"severity":       res.Severity,
				"description":    res.Description,
				"meta":           res.Meta,
			})
	}

	// actionlogUpsertQuery assembles (insert+on-conflict) query for replacing actionlogs
	//
	// This function is auto-generated
	actionlogUpsertQuery = func(d goqu.DialectWrapper, res *actionlogType.Action) *goqu.InsertDataset {
		var target = `,id`

		return actionlogInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"ts":             res.Timestamp,
						"actor_ip_addr":  res.ActorIPAddr,
						"actor_id":       res.ActorID,
						"request_origin": res.RequestOrigin,
						"request_id":     res.RequestID,
						"resource":       res.Resource,
						"action":         res.Action,
						"error":          res.Error,
						"severity":       res.Severity,
						"description":    res.Description,
						"meta":           res.Meta,
					},
				),
			)
	}

	// actionlogUpdateQuery assembles query for updating actionlogs
	//
	// This function is auto-generated
	actionlogUpdateQuery = func(d goqu.DialectWrapper, res *actionlogType.Action) *goqu.UpdateDataset {
		return d.Update(actionlogTable).
			Set(goqu.Record{
				"ts":             res.Timestamp,
				"actor_ip_addr":  res.ActorIPAddr,
				"actor_id":       res.ActorID,
				"request_origin": res.RequestOrigin,
				"request_id":     res.RequestID,
				"resource":       res.Resource,
				"action":         res.Action,
				"error":          res.Error,
				"severity":       res.Severity,
				"description":    res.Description,
				"meta":           res.Meta,
			}).
			Where(actionlogPrimaryKeys(res))
	}

	// actionlogDeleteQuery assembles delete query for removing actionlogs
	//
	// This function is auto-generated
	actionlogDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(actionlogTable).Where(ee...)
	}

	// actionlogDeleteQuery assembles delete query for removing actionlogs
	//
	// This function is auto-generated
	actionlogTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(actionlogTable)
	}

	// actionlogPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	actionlogPrimaryKeys = func(res *actionlogType.Action) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// apigwFilterTable represents apigwFilters store table
	//
	// This value is auto-generated
	apigwFilterTable = goqu.T("apigw_filters")

	// apigwFilterSelectQuery assembles select query for fetching apigwFilters
	//
	// This function is auto-generated
	apigwFilterSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"rel_route",
			"weight",
			"kind",
			"ref",
			"enabled",
			"params",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(apigwFilterTable)
	}

	// apigwFilterInsertQuery assembles query inserting apigwFilters
	//
	// This function is auto-generated
	apigwFilterInsertQuery = func(d goqu.DialectWrapper, res *systemType.ApigwFilter) *goqu.InsertDataset {
		return d.Insert(apigwFilterTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"rel_route":  res.Route,
				"weight":     res.Weight,
				"kind":       res.Kind,
				"ref":        res.Ref,
				"enabled":    res.Enabled,
				"params":     res.Params,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			})
	}

	// apigwFilterUpsertQuery assembles (insert+on-conflict) query for replacing apigwFilters
	//
	// This function is auto-generated
	apigwFilterUpsertQuery = func(d goqu.DialectWrapper, res *systemType.ApigwFilter) *goqu.InsertDataset {
		var target = `,id`

		return apigwFilterInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"rel_route":  res.Route,
						"weight":     res.Weight,
						"kind":       res.Kind,
						"ref":        res.Ref,
						"enabled":    res.Enabled,
						"params":     res.Params,
						"created_at": res.CreatedAt,
						"updated_at": res.UpdatedAt,
						"deleted_at": res.DeletedAt,
						"created_by": res.CreatedBy,
						"updated_by": res.UpdatedBy,
						"deleted_by": res.DeletedBy,
					},
				),
			)
	}

	// apigwFilterUpdateQuery assembles query for updating apigwFilters
	//
	// This function is auto-generated
	apigwFilterUpdateQuery = func(d goqu.DialectWrapper, res *systemType.ApigwFilter) *goqu.UpdateDataset {
		return d.Update(apigwFilterTable).
			Set(goqu.Record{
				"rel_route":  res.Route,
				"weight":     res.Weight,
				"kind":       res.Kind,
				"ref":        res.Ref,
				"enabled":    res.Enabled,
				"params":     res.Params,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			}).
			Where(apigwFilterPrimaryKeys(res))
	}

	// apigwFilterDeleteQuery assembles delete query for removing apigwFilters
	//
	// This function is auto-generated
	apigwFilterDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(apigwFilterTable).Where(ee...)
	}

	// apigwFilterDeleteQuery assembles delete query for removing apigwFilters
	//
	// This function is auto-generated
	apigwFilterTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(apigwFilterTable)
	}

	// apigwFilterPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	apigwFilterPrimaryKeys = func(res *systemType.ApigwFilter) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// apigwRouteTable represents apigwRoutes store table
	//
	// This value is auto-generated
	apigwRouteTable = goqu.T("apigw_routes")

	// apigwRouteSelectQuery assembles select query for fetching apigwRoutes
	//
	// This function is auto-generated
	apigwRouteSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"endpoint",
			"method",
			"enabled",
			"meta",
			"rel_group",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(apigwRouteTable)
	}

	// apigwRouteInsertQuery assembles query inserting apigwRoutes
	//
	// This function is auto-generated
	apigwRouteInsertQuery = func(d goqu.DialectWrapper, res *systemType.ApigwRoute) *goqu.InsertDataset {
		return d.Insert(apigwRouteTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"endpoint":   res.Endpoint,
				"method":     res.Method,
				"enabled":    res.Enabled,
				"meta":       res.Meta,
				"rel_group":  res.Group,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			})
	}

	// apigwRouteUpsertQuery assembles (insert+on-conflict) query for replacing apigwRoutes
	//
	// This function is auto-generated
	apigwRouteUpsertQuery = func(d goqu.DialectWrapper, res *systemType.ApigwRoute) *goqu.InsertDataset {
		var target = `,id`

		return apigwRouteInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"endpoint":   res.Endpoint,
						"method":     res.Method,
						"enabled":    res.Enabled,
						"meta":       res.Meta,
						"rel_group":  res.Group,
						"created_at": res.CreatedAt,
						"updated_at": res.UpdatedAt,
						"deleted_at": res.DeletedAt,
						"created_by": res.CreatedBy,
						"updated_by": res.UpdatedBy,
						"deleted_by": res.DeletedBy,
					},
				),
			)
	}

	// apigwRouteUpdateQuery assembles query for updating apigwRoutes
	//
	// This function is auto-generated
	apigwRouteUpdateQuery = func(d goqu.DialectWrapper, res *systemType.ApigwRoute) *goqu.UpdateDataset {
		return d.Update(apigwRouteTable).
			Set(goqu.Record{
				"endpoint":   res.Endpoint,
				"method":     res.Method,
				"enabled":    res.Enabled,
				"meta":       res.Meta,
				"rel_group":  res.Group,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			}).
			Where(apigwRoutePrimaryKeys(res))
	}

	// apigwRouteDeleteQuery assembles delete query for removing apigwRoutes
	//
	// This function is auto-generated
	apigwRouteDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(apigwRouteTable).Where(ee...)
	}

	// apigwRouteDeleteQuery assembles delete query for removing apigwRoutes
	//
	// This function is auto-generated
	apigwRouteTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(apigwRouteTable)
	}

	// apigwRoutePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	apigwRoutePrimaryKeys = func(res *systemType.ApigwRoute) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// applicationTable represents applications store table
	//
	// This value is auto-generated
	applicationTable = goqu.T("applications")

	// applicationSelectQuery assembles select query for fetching applications
	//
	// This function is auto-generated
	applicationSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"name",
			"enabled",
			"weight",
			"unify",
			"rel_owner",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(applicationTable)
	}

	// applicationInsertQuery assembles query inserting applications
	//
	// This function is auto-generated
	applicationInsertQuery = func(d goqu.DialectWrapper, res *systemType.Application) *goqu.InsertDataset {
		return d.Insert(applicationTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"name":       res.Name,
				"enabled":    res.Enabled,
				"weight":     res.Weight,
				"unify":      res.Unify,
				"rel_owner":  res.OwnerID,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
			})
	}

	// applicationUpsertQuery assembles (insert+on-conflict) query for replacing applications
	//
	// This function is auto-generated
	applicationUpsertQuery = func(d goqu.DialectWrapper, res *systemType.Application) *goqu.InsertDataset {
		var target = `,id`

		return applicationInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"name":       res.Name,
						"enabled":    res.Enabled,
						"weight":     res.Weight,
						"unify":      res.Unify,
						"rel_owner":  res.OwnerID,
						"created_at": res.CreatedAt,
						"updated_at": res.UpdatedAt,
						"deleted_at": res.DeletedAt,
					},
				),
			)
	}

	// applicationUpdateQuery assembles query for updating applications
	//
	// This function is auto-generated
	applicationUpdateQuery = func(d goqu.DialectWrapper, res *systemType.Application) *goqu.UpdateDataset {
		return d.Update(applicationTable).
			Set(goqu.Record{
				"name":       res.Name,
				"enabled":    res.Enabled,
				"weight":     res.Weight,
				"unify":      res.Unify,
				"rel_owner":  res.OwnerID,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
			}).
			Where(applicationPrimaryKeys(res))
	}

	// applicationDeleteQuery assembles delete query for removing applications
	//
	// This function is auto-generated
	applicationDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(applicationTable).Where(ee...)
	}

	// applicationDeleteQuery assembles delete query for removing applications
	//
	// This function is auto-generated
	applicationTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(applicationTable)
	}

	// applicationPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	applicationPrimaryKeys = func(res *systemType.Application) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// attachmentTable represents attachments store table
	//
	// This value is auto-generated
	attachmentTable = goqu.T("attachments")

	// attachmentSelectQuery assembles select query for fetching attachments
	//
	// This function is auto-generated
	attachmentSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"rel_owner",
			"kind",
			"url",
			"preview_url",
			"name",
			"meta",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(attachmentTable)
	}

	// attachmentInsertQuery assembles query inserting attachments
	//
	// This function is auto-generated
	attachmentInsertQuery = func(d goqu.DialectWrapper, res *systemType.Attachment) *goqu.InsertDataset {
		return d.Insert(attachmentTable).
			Rows(goqu.Record{
				"id":          res.ID,
				"rel_owner":   res.OwnerID,
				"kind":        res.Kind,
				"url":         res.Url,
				"preview_url": res.PreviewUrl,
				"name":        res.Name,
				"meta":        res.Meta,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
				"deleted_at":  res.DeletedAt,
			})
	}

	// attachmentUpsertQuery assembles (insert+on-conflict) query for replacing attachments
	//
	// This function is auto-generated
	attachmentUpsertQuery = func(d goqu.DialectWrapper, res *systemType.Attachment) *goqu.InsertDataset {
		var target = `,id`

		return attachmentInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"rel_owner":   res.OwnerID,
						"kind":        res.Kind,
						"url":         res.Url,
						"preview_url": res.PreviewUrl,
						"name":        res.Name,
						"meta":        res.Meta,
						"created_at":  res.CreatedAt,
						"updated_at":  res.UpdatedAt,
						"deleted_at":  res.DeletedAt,
					},
				),
			)
	}

	// attachmentUpdateQuery assembles query for updating attachments
	//
	// This function is auto-generated
	attachmentUpdateQuery = func(d goqu.DialectWrapper, res *systemType.Attachment) *goqu.UpdateDataset {
		return d.Update(attachmentTable).
			Set(goqu.Record{
				"rel_owner":   res.OwnerID,
				"kind":        res.Kind,
				"url":         res.Url,
				"preview_url": res.PreviewUrl,
				"name":        res.Name,
				"meta":        res.Meta,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
				"deleted_at":  res.DeletedAt,
			}).
			Where(attachmentPrimaryKeys(res))
	}

	// attachmentDeleteQuery assembles delete query for removing attachments
	//
	// This function is auto-generated
	attachmentDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(attachmentTable).Where(ee...)
	}

	// attachmentDeleteQuery assembles delete query for removing attachments
	//
	// This function is auto-generated
	attachmentTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(attachmentTable)
	}

	// attachmentPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	attachmentPrimaryKeys = func(res *systemType.Attachment) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// authClientTable represents authClients store table
	//
	// This value is auto-generated
	authClientTable = goqu.T("auth_clients")

	// authClientSelectQuery assembles select query for fetching authClients
	//
	// This function is auto-generated
	authClientSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"handle",
			"meta",
			"secret",
			"scope",
			"valid_grant",
			"redirect_uri",
			"enabled",
			"trusted",
			"valid_from",
			"expires_at",
			"security",
			"owned_by",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(authClientTable)
	}

	// authClientInsertQuery assembles query inserting authClients
	//
	// This function is auto-generated
	authClientInsertQuery = func(d goqu.DialectWrapper, res *systemType.AuthClient) *goqu.InsertDataset {
		return d.Insert(authClientTable).
			Rows(goqu.Record{
				"id":           res.ID,
				"handle":       res.Handle,
				"meta":         res.Meta,
				"secret":       res.Secret,
				"scope":        res.Scope,
				"valid_grant":  res.ValidGrant,
				"redirect_uri": res.RedirectURI,
				"enabled":      res.Enabled,
				"trusted":      res.Trusted,
				"valid_from":   res.ValidFrom,
				"expires_at":   res.ExpiresAt,
				"security":     res.Security,
				"owned_by":     res.OwnedBy,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"deleted_at":   res.DeletedAt,
				"created_by":   res.CreatedBy,
				"updated_by":   res.UpdatedBy,
				"deleted_by":   res.DeletedBy,
			})
	}

	// authClientUpsertQuery assembles (insert+on-conflict) query for replacing authClients
	//
	// This function is auto-generated
	authClientUpsertQuery = func(d goqu.DialectWrapper, res *systemType.AuthClient) *goqu.InsertDataset {
		var target = `,id`

		return authClientInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"handle":       res.Handle,
						"meta":         res.Meta,
						"secret":       res.Secret,
						"scope":        res.Scope,
						"valid_grant":  res.ValidGrant,
						"redirect_uri": res.RedirectURI,
						"enabled":      res.Enabled,
						"trusted":      res.Trusted,
						"valid_from":   res.ValidFrom,
						"expires_at":   res.ExpiresAt,
						"security":     res.Security,
						"owned_by":     res.OwnedBy,
						"created_at":   res.CreatedAt,
						"updated_at":   res.UpdatedAt,
						"deleted_at":   res.DeletedAt,
						"created_by":   res.CreatedBy,
						"updated_by":   res.UpdatedBy,
						"deleted_by":   res.DeletedBy,
					},
				),
			)
	}

	// authClientUpdateQuery assembles query for updating authClients
	//
	// This function is auto-generated
	authClientUpdateQuery = func(d goqu.DialectWrapper, res *systemType.AuthClient) *goqu.UpdateDataset {
		return d.Update(authClientTable).
			Set(goqu.Record{
				"handle":       res.Handle,
				"meta":         res.Meta,
				"secret":       res.Secret,
				"scope":        res.Scope,
				"valid_grant":  res.ValidGrant,
				"redirect_uri": res.RedirectURI,
				"enabled":      res.Enabled,
				"trusted":      res.Trusted,
				"valid_from":   res.ValidFrom,
				"expires_at":   res.ExpiresAt,
				"security":     res.Security,
				"owned_by":     res.OwnedBy,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"deleted_at":   res.DeletedAt,
				"created_by":   res.CreatedBy,
				"updated_by":   res.UpdatedBy,
				"deleted_by":   res.DeletedBy,
			}).
			Where(authClientPrimaryKeys(res))
	}

	// authClientDeleteQuery assembles delete query for removing authClients
	//
	// This function is auto-generated
	authClientDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(authClientTable).Where(ee...)
	}

	// authClientDeleteQuery assembles delete query for removing authClients
	//
	// This function is auto-generated
	authClientTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(authClientTable)
	}

	// authClientPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	authClientPrimaryKeys = func(res *systemType.AuthClient) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// authConfirmedClientTable represents authConfirmedClients store table
	//
	// This value is auto-generated
	authConfirmedClientTable = goqu.T("auth_confirmed_clients")

	// authConfirmedClientSelectQuery assembles select query for fetching authConfirmedClients
	//
	// This function is auto-generated
	authConfirmedClientSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"rel_user",
			"rel_client",
			"confirmed_at",
		).From(authConfirmedClientTable)
	}

	// authConfirmedClientInsertQuery assembles query inserting authConfirmedClients
	//
	// This function is auto-generated
	authConfirmedClientInsertQuery = func(d goqu.DialectWrapper, res *systemType.AuthConfirmedClient) *goqu.InsertDataset {
		return d.Insert(authConfirmedClientTable).
			Rows(goqu.Record{
				"rel_user":     res.UserID,
				"rel_client":   res.ClientID,
				"confirmed_at": res.ConfirmedAt,
			})
	}

	// authConfirmedClientUpsertQuery assembles (insert+on-conflict) query for replacing authConfirmedClients
	//
	// This function is auto-generated
	authConfirmedClientUpsertQuery = func(d goqu.DialectWrapper, res *systemType.AuthConfirmedClient) *goqu.InsertDataset {
		var target = `,rel_user,rel_client`

		return authConfirmedClientInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"confirmed_at": res.ConfirmedAt,
					},
				),
			)
	}

	// authConfirmedClientUpdateQuery assembles query for updating authConfirmedClients
	//
	// This function is auto-generated
	authConfirmedClientUpdateQuery = func(d goqu.DialectWrapper, res *systemType.AuthConfirmedClient) *goqu.UpdateDataset {
		return d.Update(authConfirmedClientTable).
			Set(goqu.Record{
				"confirmed_at": res.ConfirmedAt,
			}).
			Where(authConfirmedClientPrimaryKeys(res))
	}

	// authConfirmedClientDeleteQuery assembles delete query for removing authConfirmedClients
	//
	// This function is auto-generated
	authConfirmedClientDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(authConfirmedClientTable).Where(ee...)
	}

	// authConfirmedClientDeleteQuery assembles delete query for removing authConfirmedClients
	//
	// This function is auto-generated
	authConfirmedClientTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(authConfirmedClientTable)
	}

	// authConfirmedClientPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	authConfirmedClientPrimaryKeys = func(res *systemType.AuthConfirmedClient) goqu.Ex {
		return goqu.Ex{
			"rel_user":   res.UserID,
			"rel_client": res.ClientID,
		}
	}

	// authOa2tokenTable represents authOa2tokens store table
	//
	// This value is auto-generated
	authOa2tokenTable = goqu.T("auth_oa2tokens")

	// authOa2tokenSelectQuery assembles select query for fetching authOa2tokens
	//
	// This function is auto-generated
	authOa2tokenSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"code",
			"access",
			"refresh",
			"data",
			"remote_addr",
			"user_agent",
			"rel_client",
			"rel_user",
			"created_at",
			"expires_at",
		).From(authOa2tokenTable)
	}

	// authOa2tokenInsertQuery assembles query inserting authOa2tokens
	//
	// This function is auto-generated
	authOa2tokenInsertQuery = func(d goqu.DialectWrapper, res *systemType.AuthOa2token) *goqu.InsertDataset {
		return d.Insert(authOa2tokenTable).
			Rows(goqu.Record{
				"id":          res.ID,
				"code":        res.Code,
				"access":      res.Access,
				"refresh":     res.Refresh,
				"data":        res.Data,
				"remote_addr": res.RemoteAddr,
				"user_agent":  res.UserAgent,
				"rel_client":  res.ClientID,
				"rel_user":    res.UserID,
				"created_at":  res.CreatedAt,
				"expires_at":  res.ExpiresAt,
			})
	}

	// authOa2tokenUpsertQuery assembles (insert+on-conflict) query for replacing authOa2tokens
	//
	// This function is auto-generated
	authOa2tokenUpsertQuery = func(d goqu.DialectWrapper, res *systemType.AuthOa2token) *goqu.InsertDataset {
		var target = `,id`

		return authOa2tokenInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"code":        res.Code,
						"access":      res.Access,
						"refresh":     res.Refresh,
						"data":        res.Data,
						"remote_addr": res.RemoteAddr,
						"user_agent":  res.UserAgent,
						"rel_client":  res.ClientID,
						"rel_user":    res.UserID,
						"created_at":  res.CreatedAt,
						"expires_at":  res.ExpiresAt,
					},
				),
			)
	}

	// authOa2tokenUpdateQuery assembles query for updating authOa2tokens
	//
	// This function is auto-generated
	authOa2tokenUpdateQuery = func(d goqu.DialectWrapper, res *systemType.AuthOa2token) *goqu.UpdateDataset {
		return d.Update(authOa2tokenTable).
			Set(goqu.Record{
				"code":        res.Code,
				"access":      res.Access,
				"refresh":     res.Refresh,
				"data":        res.Data,
				"remote_addr": res.RemoteAddr,
				"user_agent":  res.UserAgent,
				"rel_client":  res.ClientID,
				"rel_user":    res.UserID,
				"created_at":  res.CreatedAt,
				"expires_at":  res.ExpiresAt,
			}).
			Where(authOa2tokenPrimaryKeys(res))
	}

	// authOa2tokenDeleteQuery assembles delete query for removing authOa2tokens
	//
	// This function is auto-generated
	authOa2tokenDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(authOa2tokenTable).Where(ee...)
	}

	// authOa2tokenDeleteQuery assembles delete query for removing authOa2tokens
	//
	// This function is auto-generated
	authOa2tokenTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(authOa2tokenTable)
	}

	// authOa2tokenPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	authOa2tokenPrimaryKeys = func(res *systemType.AuthOa2token) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// authSessionTable represents authSessions store table
	//
	// This value is auto-generated
	authSessionTable = goqu.T("auth_sessions")

	// authSessionSelectQuery assembles select query for fetching authSessions
	//
	// This function is auto-generated
	authSessionSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"data",
			"rel_user",
			"remote_addr",
			"user_agent",
			"expires_at",
			"created_at",
		).From(authSessionTable)
	}

	// authSessionInsertQuery assembles query inserting authSessions
	//
	// This function is auto-generated
	authSessionInsertQuery = func(d goqu.DialectWrapper, res *systemType.AuthSession) *goqu.InsertDataset {
		return d.Insert(authSessionTable).
			Rows(goqu.Record{
				"id":          res.ID,
				"data":        res.Data,
				"rel_user":    res.UserID,
				"remote_addr": res.RemoteAddr,
				"user_agent":  res.UserAgent,
				"expires_at":  res.ExpiresAt,
				"created_at":  res.CreatedAt,
			})
	}

	// authSessionUpsertQuery assembles (insert+on-conflict) query for replacing authSessions
	//
	// This function is auto-generated
	authSessionUpsertQuery = func(d goqu.DialectWrapper, res *systemType.AuthSession) *goqu.InsertDataset {
		var target = `,id`

		return authSessionInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"data":        res.Data,
						"rel_user":    res.UserID,
						"remote_addr": res.RemoteAddr,
						"user_agent":  res.UserAgent,
						"expires_at":  res.ExpiresAt,
						"created_at":  res.CreatedAt,
					},
				),
			)
	}

	// authSessionUpdateQuery assembles query for updating authSessions
	//
	// This function is auto-generated
	authSessionUpdateQuery = func(d goqu.DialectWrapper, res *systemType.AuthSession) *goqu.UpdateDataset {
		return d.Update(authSessionTable).
			Set(goqu.Record{
				"data":        res.Data,
				"rel_user":    res.UserID,
				"remote_addr": res.RemoteAddr,
				"user_agent":  res.UserAgent,
				"expires_at":  res.ExpiresAt,
				"created_at":  res.CreatedAt,
			}).
			Where(authSessionPrimaryKeys(res))
	}

	// authSessionDeleteQuery assembles delete query for removing authSessions
	//
	// This function is auto-generated
	authSessionDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(authSessionTable).Where(ee...)
	}

	// authSessionDeleteQuery assembles delete query for removing authSessions
	//
	// This function is auto-generated
	authSessionTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(authSessionTable)
	}

	// authSessionPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	authSessionPrimaryKeys = func(res *systemType.AuthSession) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// automationSessionTable represents automationSessions store table
	//
	// This value is auto-generated
	automationSessionTable = goqu.T("automation_sessions")

	// automationSessionSelectQuery assembles select query for fetching automationSessions
	//
	// This function is auto-generated
	automationSessionSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"rel_workflow",
			"status",
			"event_type",
			"resource_type",
			"input",
			"output",
			"stacktrace",
			"created_by",
			"created_at",
			"purge_at",
			"suspended_at",
			"completed_at",
			"error",
		).From(automationSessionTable)
	}

	// automationSessionInsertQuery assembles query inserting automationSessions
	//
	// This function is auto-generated
	automationSessionInsertQuery = func(d goqu.DialectWrapper, res *automationType.Session) *goqu.InsertDataset {
		return d.Insert(automationSessionTable).
			Rows(goqu.Record{
				"id":            res.ID,
				"rel_workflow":  res.WorkflowID,
				"status":        res.Status,
				"event_type":    res.EventType,
				"resource_type": res.ResourceType,
				"input":         res.Input,
				"output":        res.Output,
				"stacktrace":    res.Stacktrace,
				"created_by":    res.CreatedBy,
				"created_at":    res.CreatedAt,
				"purge_at":      res.PurgeAt,
				"suspended_at":  res.SuspendedAt,
				"completed_at":  res.CompletedAt,
				"error":         res.Error,
			})
	}

	// automationSessionUpsertQuery assembles (insert+on-conflict) query for replacing automationSessions
	//
	// This function is auto-generated
	automationSessionUpsertQuery = func(d goqu.DialectWrapper, res *automationType.Session) *goqu.InsertDataset {
		var target = `,id`

		return automationSessionInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"rel_workflow":  res.WorkflowID,
						"status":        res.Status,
						"event_type":    res.EventType,
						"resource_type": res.ResourceType,
						"input":         res.Input,
						"output":        res.Output,
						"stacktrace":    res.Stacktrace,
						"created_by":    res.CreatedBy,
						"created_at":    res.CreatedAt,
						"purge_at":      res.PurgeAt,
						"suspended_at":  res.SuspendedAt,
						"completed_at":  res.CompletedAt,
						"error":         res.Error,
					},
				),
			)
	}

	// automationSessionUpdateQuery assembles query for updating automationSessions
	//
	// This function is auto-generated
	automationSessionUpdateQuery = func(d goqu.DialectWrapper, res *automationType.Session) *goqu.UpdateDataset {
		return d.Update(automationSessionTable).
			Set(goqu.Record{
				"rel_workflow":  res.WorkflowID,
				"status":        res.Status,
				"event_type":    res.EventType,
				"resource_type": res.ResourceType,
				"input":         res.Input,
				"output":        res.Output,
				"stacktrace":    res.Stacktrace,
				"created_by":    res.CreatedBy,
				"created_at":    res.CreatedAt,
				"purge_at":      res.PurgeAt,
				"suspended_at":  res.SuspendedAt,
				"completed_at":  res.CompletedAt,
				"error":         res.Error,
			}).
			Where(automationSessionPrimaryKeys(res))
	}

	// automationSessionDeleteQuery assembles delete query for removing automationSessions
	//
	// This function is auto-generated
	automationSessionDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(automationSessionTable).Where(ee...)
	}

	// automationSessionDeleteQuery assembles delete query for removing automationSessions
	//
	// This function is auto-generated
	automationSessionTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(automationSessionTable)
	}

	// automationSessionPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	automationSessionPrimaryKeys = func(res *automationType.Session) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// automationTriggerTable represents automationTriggers store table
	//
	// This value is auto-generated
	automationTriggerTable = goqu.T("automation_triggers")

	// automationTriggerSelectQuery assembles select query for fetching automationTriggers
	//
	// This function is auto-generated
	automationTriggerSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"rel_workflow",
			"rel_step",
			"enabled",
			"meta",
			"resource_type",
			"event_type",
			"constraints",
			"input",
			"owned_by",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(automationTriggerTable)
	}

	// automationTriggerInsertQuery assembles query inserting automationTriggers
	//
	// This function is auto-generated
	automationTriggerInsertQuery = func(d goqu.DialectWrapper, res *automationType.Trigger) *goqu.InsertDataset {
		return d.Insert(automationTriggerTable).
			Rows(goqu.Record{
				"id":            res.ID,
				"rel_workflow":  res.WorkflowID,
				"rel_step":      res.StepID,
				"enabled":       res.Enabled,
				"meta":          res.Meta,
				"resource_type": res.ResourceType,
				"event_type":    res.EventType,
				"constraints":   res.Constraints,
				"input":         res.Input,
				"owned_by":      res.OwnedBy,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
				"created_by":    res.CreatedBy,
				"updated_by":    res.UpdatedBy,
				"deleted_by":    res.DeletedBy,
			})
	}

	// automationTriggerUpsertQuery assembles (insert+on-conflict) query for replacing automationTriggers
	//
	// This function is auto-generated
	automationTriggerUpsertQuery = func(d goqu.DialectWrapper, res *automationType.Trigger) *goqu.InsertDataset {
		var target = `,id`

		return automationTriggerInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"rel_workflow":  res.WorkflowID,
						"rel_step":      res.StepID,
						"enabled":       res.Enabled,
						"meta":          res.Meta,
						"resource_type": res.ResourceType,
						"event_type":    res.EventType,
						"constraints":   res.Constraints,
						"input":         res.Input,
						"owned_by":      res.OwnedBy,
						"created_at":    res.CreatedAt,
						"updated_at":    res.UpdatedAt,
						"deleted_at":    res.DeletedAt,
						"created_by":    res.CreatedBy,
						"updated_by":    res.UpdatedBy,
						"deleted_by":    res.DeletedBy,
					},
				),
			)
	}

	// automationTriggerUpdateQuery assembles query for updating automationTriggers
	//
	// This function is auto-generated
	automationTriggerUpdateQuery = func(d goqu.DialectWrapper, res *automationType.Trigger) *goqu.UpdateDataset {
		return d.Update(automationTriggerTable).
			Set(goqu.Record{
				"rel_workflow":  res.WorkflowID,
				"rel_step":      res.StepID,
				"enabled":       res.Enabled,
				"meta":          res.Meta,
				"resource_type": res.ResourceType,
				"event_type":    res.EventType,
				"constraints":   res.Constraints,
				"input":         res.Input,
				"owned_by":      res.OwnedBy,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
				"created_by":    res.CreatedBy,
				"updated_by":    res.UpdatedBy,
				"deleted_by":    res.DeletedBy,
			}).
			Where(automationTriggerPrimaryKeys(res))
	}

	// automationTriggerDeleteQuery assembles delete query for removing automationTriggers
	//
	// This function is auto-generated
	automationTriggerDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(automationTriggerTable).Where(ee...)
	}

	// automationTriggerDeleteQuery assembles delete query for removing automationTriggers
	//
	// This function is auto-generated
	automationTriggerTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(automationTriggerTable)
	}

	// automationTriggerPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	automationTriggerPrimaryKeys = func(res *automationType.Trigger) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// automationWorkflowTable represents automationWorkflows store table
	//
	// This value is auto-generated
	automationWorkflowTable = goqu.T("automation_workflows")

	// automationWorkflowSelectQuery assembles select query for fetching automationWorkflows
	//
	// This function is auto-generated
	automationWorkflowSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"handle",
			"meta",
			"enabled",
			"trace",
			"keep_sessions",
			"scope",
			"steps",
			"paths",
			"issues",
			"run_as",
			"owned_by",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(automationWorkflowTable)
	}

	// automationWorkflowInsertQuery assembles query inserting automationWorkflows
	//
	// This function is auto-generated
	automationWorkflowInsertQuery = func(d goqu.DialectWrapper, res *automationType.Workflow) *goqu.InsertDataset {
		return d.Insert(automationWorkflowTable).
			Rows(goqu.Record{
				"id":            res.ID,
				"handle":        res.Handle,
				"meta":          res.Meta,
				"enabled":       res.Enabled,
				"trace":         res.Trace,
				"keep_sessions": res.KeepSessions,
				"scope":         res.Scope,
				"steps":         res.Steps,
				"paths":         res.Paths,
				"issues":        res.Issues,
				"run_as":        res.RunAs,
				"owned_by":      res.OwnedBy,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
				"created_by":    res.CreatedBy,
				"updated_by":    res.UpdatedBy,
				"deleted_by":    res.DeletedBy,
			})
	}

	// automationWorkflowUpsertQuery assembles (insert+on-conflict) query for replacing automationWorkflows
	//
	// This function is auto-generated
	automationWorkflowUpsertQuery = func(d goqu.DialectWrapper, res *automationType.Workflow) *goqu.InsertDataset {
		var target = `,id`

		return automationWorkflowInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"handle":        res.Handle,
						"meta":          res.Meta,
						"enabled":       res.Enabled,
						"trace":         res.Trace,
						"keep_sessions": res.KeepSessions,
						"scope":         res.Scope,
						"steps":         res.Steps,
						"paths":         res.Paths,
						"issues":        res.Issues,
						"run_as":        res.RunAs,
						"owned_by":      res.OwnedBy,
						"created_at":    res.CreatedAt,
						"updated_at":    res.UpdatedAt,
						"deleted_at":    res.DeletedAt,
						"created_by":    res.CreatedBy,
						"updated_by":    res.UpdatedBy,
						"deleted_by":    res.DeletedBy,
					},
				),
			)
	}

	// automationWorkflowUpdateQuery assembles query for updating automationWorkflows
	//
	// This function is auto-generated
	automationWorkflowUpdateQuery = func(d goqu.DialectWrapper, res *automationType.Workflow) *goqu.UpdateDataset {
		return d.Update(automationWorkflowTable).
			Set(goqu.Record{
				"handle":        res.Handle,
				"meta":          res.Meta,
				"enabled":       res.Enabled,
				"trace":         res.Trace,
				"keep_sessions": res.KeepSessions,
				"scope":         res.Scope,
				"steps":         res.Steps,
				"paths":         res.Paths,
				"issues":        res.Issues,
				"run_as":        res.RunAs,
				"owned_by":      res.OwnedBy,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
				"created_by":    res.CreatedBy,
				"updated_by":    res.UpdatedBy,
				"deleted_by":    res.DeletedBy,
			}).
			Where(automationWorkflowPrimaryKeys(res))
	}

	// automationWorkflowDeleteQuery assembles delete query for removing automationWorkflows
	//
	// This function is auto-generated
	automationWorkflowDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(automationWorkflowTable).Where(ee...)
	}

	// automationWorkflowDeleteQuery assembles delete query for removing automationWorkflows
	//
	// This function is auto-generated
	automationWorkflowTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(automationWorkflowTable)
	}

	// automationWorkflowPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	automationWorkflowPrimaryKeys = func(res *automationType.Workflow) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311ActorProfileTable represents city311ActorProfiles store table
	//
	// This value is auto-generated
	city311ActorProfileTable = goqu.T("compose_city311_actor_profile")

	// city311ActorProfileSelectQuery assembles select query for fetching city311ActorProfiles
	//
	// This function is auto-generated
	city311ActorProfileSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"application_roles",
			"department",
			"districts",
			"created_at",
			"updated_at",
		).From(city311ActorProfileTable)
	}

	// city311ActorProfileInsertQuery assembles query inserting city311ActorProfiles
	//
	// This function is auto-generated
	city311ActorProfileInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311ActorProfile) *goqu.InsertDataset {
		return d.Insert(city311ActorProfileTable).
			Rows(goqu.Record{
				"id":                res.ID,
				"application_roles": res.ApplicationRoles,
				"department":        res.Department,
				"districts":         res.Districts,
				"created_at":        res.CreatedAt,
				"updated_at":        res.UpdatedAt,
			})
	}

	// city311ActorProfileUpsertQuery assembles (insert+on-conflict) query for replacing city311ActorProfiles
	//
	// This function is auto-generated
	city311ActorProfileUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311ActorProfile) *goqu.InsertDataset {
		var target = `,id`

		return city311ActorProfileInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"application_roles": res.ApplicationRoles,
						"department":        res.Department,
						"districts":         res.Districts,
						"created_at":        res.CreatedAt,
						"updated_at":        res.UpdatedAt,
					},
				),
			)
	}

	// city311ActorProfileUpdateQuery assembles query for updating city311ActorProfiles
	//
	// This function is auto-generated
	city311ActorProfileUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311ActorProfile) *goqu.UpdateDataset {
		return d.Update(city311ActorProfileTable).
			Set(goqu.Record{
				"application_roles": res.ApplicationRoles,
				"department":        res.Department,
				"districts":         res.Districts,
				"created_at":        res.CreatedAt,
				"updated_at":        res.UpdatedAt,
			}).
			Where(city311ActorProfilePrimaryKeys(res))
	}

	// city311ActorProfileDeleteQuery assembles delete query for removing city311ActorProfiles
	//
	// This function is auto-generated
	city311ActorProfileDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311ActorProfileTable).Where(ee...)
	}

	// city311ActorProfileDeleteQuery assembles delete query for removing city311ActorProfiles
	//
	// This function is auto-generated
	city311ActorProfileTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311ActorProfileTable)
	}

	// city311ActorProfilePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311ActorProfilePrimaryKeys = func(res *composeType.City311ActorProfile) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311AuditEventTable represents city311AuditEvents store table
	//
	// This value is auto-generated
	city311AuditEventTable = goqu.T("compose_city311_audit_event")

	// city311AuditEventSelectQuery assembles select query for fetching city311AuditEvents
	//
	// This function is auto-generated
	city311AuditEventSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"request_id",
			"entity_type",
			"entity_id",
			"event_type",
			"actor_type",
			"actor_id",
			"source_channel",
			"before",
			"after",
			"created_at",
		).From(city311AuditEventTable)
	}

	// city311AuditEventInsertQuery assembles query inserting city311AuditEvents
	//
	// This function is auto-generated
	city311AuditEventInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311AuditEvent) *goqu.InsertDataset {
		return d.Insert(city311AuditEventTable).
			Rows(goqu.Record{
				"id":             res.ID,
				"request_id":     res.RequestID,
				"entity_type":    res.EntityType,
				"entity_id":      res.EntityID,
				"event_type":     res.EventType,
				"actor_type":     res.ActorType,
				"actor_id":       res.ActorID,
				"source_channel": res.SourceChannel,
				"before":         res.Before,
				"after":          res.After,
				"created_at":     res.CreatedAt,
			})
	}

	// city311AuditEventUpsertQuery assembles (insert+on-conflict) query for replacing city311AuditEvents
	//
	// This function is auto-generated
	city311AuditEventUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311AuditEvent) *goqu.InsertDataset {
		var target = `,id`

		return city311AuditEventInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"request_id":     res.RequestID,
						"entity_type":    res.EntityType,
						"entity_id":      res.EntityID,
						"event_type":     res.EventType,
						"actor_type":     res.ActorType,
						"actor_id":       res.ActorID,
						"source_channel": res.SourceChannel,
						"before":         res.Before,
						"after":          res.After,
						"created_at":     res.CreatedAt,
					},
				),
			)
	}

	// city311AuditEventUpdateQuery assembles query for updating city311AuditEvents
	//
	// This function is auto-generated
	city311AuditEventUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311AuditEvent) *goqu.UpdateDataset {
		return d.Update(city311AuditEventTable).
			Set(goqu.Record{
				"request_id":     res.RequestID,
				"entity_type":    res.EntityType,
				"entity_id":      res.EntityID,
				"event_type":     res.EventType,
				"actor_type":     res.ActorType,
				"actor_id":       res.ActorID,
				"source_channel": res.SourceChannel,
				"before":         res.Before,
				"after":          res.After,
				"created_at":     res.CreatedAt,
			}).
			Where(city311AuditEventPrimaryKeys(res))
	}

	// city311AuditEventDeleteQuery assembles delete query for removing city311AuditEvents
	//
	// This function is auto-generated
	city311AuditEventDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311AuditEventTable).Where(ee...)
	}

	// city311AuditEventDeleteQuery assembles delete query for removing city311AuditEvents
	//
	// This function is auto-generated
	city311AuditEventTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311AuditEventTable)
	}

	// city311AuditEventPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311AuditEventPrimaryKeys = func(res *composeType.City311AuditEvent) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311ConfigurationRevisionTable represents city311ConfigurationRevisions store table
	//
	// This value is auto-generated
	city311ConfigurationRevisionTable = goqu.T("compose_city311_configuration_revision")

	// city311ConfigurationRevisionSelectQuery assembles select query for fetching city311ConfigurationRevisions
	//
	// This function is auto-generated
	city311ConfigurationRevisionSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"resource_type",
			"resource_key",
			"language",
			"payload",
			"version",
			"published",
			"created_at",
		).From(city311ConfigurationRevisionTable)
	}

	// city311ConfigurationRevisionInsertQuery assembles query inserting city311ConfigurationRevisions
	//
	// This function is auto-generated
	city311ConfigurationRevisionInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311ConfigurationRevision) *goqu.InsertDataset {
		return d.Insert(city311ConfigurationRevisionTable).
			Rows(goqu.Record{
				"id":            res.ID,
				"resource_type": res.ResourceType,
				"resource_key":  res.ResourceKey,
				"language":      res.Language,
				"payload":       res.Payload,
				"version":       res.Version,
				"published":     res.Published,
				"created_at":    res.CreatedAt,
			})
	}

	// city311ConfigurationRevisionUpsertQuery assembles (insert+on-conflict) query for replacing city311ConfigurationRevisions
	//
	// This function is auto-generated
	city311ConfigurationRevisionUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311ConfigurationRevision) *goqu.InsertDataset {
		var target = `,id`

		return city311ConfigurationRevisionInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"resource_type": res.ResourceType,
						"resource_key":  res.ResourceKey,
						"language":      res.Language,
						"payload":       res.Payload,
						"version":       res.Version,
						"published":     res.Published,
						"created_at":    res.CreatedAt,
					},
				),
			)
	}

	// city311ConfigurationRevisionUpdateQuery assembles query for updating city311ConfigurationRevisions
	//
	// This function is auto-generated
	city311ConfigurationRevisionUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311ConfigurationRevision) *goqu.UpdateDataset {
		return d.Update(city311ConfigurationRevisionTable).
			Set(goqu.Record{
				"resource_type": res.ResourceType,
				"resource_key":  res.ResourceKey,
				"language":      res.Language,
				"payload":       res.Payload,
				"version":       res.Version,
				"published":     res.Published,
				"created_at":    res.CreatedAt,
			}).
			Where(city311ConfigurationRevisionPrimaryKeys(res))
	}

	// city311ConfigurationRevisionDeleteQuery assembles delete query for removing city311ConfigurationRevisions
	//
	// This function is auto-generated
	city311ConfigurationRevisionDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311ConfigurationRevisionTable).Where(ee...)
	}

	// city311ConfigurationRevisionDeleteQuery assembles delete query for removing city311ConfigurationRevisions
	//
	// This function is auto-generated
	city311ConfigurationRevisionTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311ConfigurationRevisionTable)
	}

	// city311ConfigurationRevisionPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311ConfigurationRevisionPrimaryKeys = func(res *composeType.City311ConfigurationRevision) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311ConstituentTable represents city311Constituents store table
	//
	// This value is auto-generated
	city311ConstituentTable = goqu.T("compose_city311_constituent")

	// city311ConstituentSelectQuery assembles select query for fetching city311Constituents
	//
	// This function is auto-generated
	city311ConstituentSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"constituent_id",
			"profile",
			"owning_department",
			"council_district",
			"created_at",
			"updated_at",
		).From(city311ConstituentTable)
	}

	// city311ConstituentInsertQuery assembles query inserting city311Constituents
	//
	// This function is auto-generated
	city311ConstituentInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311Constituent) *goqu.InsertDataset {
		return d.Insert(city311ConstituentTable).
			Rows(goqu.Record{
				"id":                res.ID,
				"constituent_id":    res.ConstituentID,
				"profile":           res.Profile,
				"owning_department": res.OwningDepartment,
				"council_district":  res.CouncilDistrict,
				"created_at":        res.CreatedAt,
				"updated_at":        res.UpdatedAt,
			})
	}

	// city311ConstituentUpsertQuery assembles (insert+on-conflict) query for replacing city311Constituents
	//
	// This function is auto-generated
	city311ConstituentUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311Constituent) *goqu.InsertDataset {
		var target = `,id`

		return city311ConstituentInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"constituent_id":    res.ConstituentID,
						"profile":           res.Profile,
						"owning_department": res.OwningDepartment,
						"council_district":  res.CouncilDistrict,
						"created_at":        res.CreatedAt,
						"updated_at":        res.UpdatedAt,
					},
				),
			)
	}

	// city311ConstituentUpdateQuery assembles query for updating city311Constituents
	//
	// This function is auto-generated
	city311ConstituentUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311Constituent) *goqu.UpdateDataset {
		return d.Update(city311ConstituentTable).
			Set(goqu.Record{
				"constituent_id":    res.ConstituentID,
				"profile":           res.Profile,
				"owning_department": res.OwningDepartment,
				"council_district":  res.CouncilDistrict,
				"created_at":        res.CreatedAt,
				"updated_at":        res.UpdatedAt,
			}).
			Where(city311ConstituentPrimaryKeys(res))
	}

	// city311ConstituentDeleteQuery assembles delete query for removing city311Constituents
	//
	// This function is auto-generated
	city311ConstituentDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311ConstituentTable).Where(ee...)
	}

	// city311ConstituentDeleteQuery assembles delete query for removing city311Constituents
	//
	// This function is auto-generated
	city311ConstituentTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311ConstituentTable)
	}

	// city311ConstituentPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311ConstituentPrimaryKeys = func(res *composeType.City311Constituent) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311IdempotencyRecordTable represents city311IdempotencyRecords store table
	//
	// This value is auto-generated
	city311IdempotencyRecordTable = goqu.T("compose_city311_idempotency")

	// city311IdempotencyRecordSelectQuery assembles select query for fetching city311IdempotencyRecords
	//
	// This function is auto-generated
	city311IdempotencyRecordSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"operation",
			"key_hash",
			"request_hash",
			"response_status",
			"response_body",
			"request_id",
			"created_at",
			"expires_at",
		).From(city311IdempotencyRecordTable)
	}

	// city311IdempotencyRecordInsertQuery assembles query inserting city311IdempotencyRecords
	//
	// This function is auto-generated
	city311IdempotencyRecordInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311IdempotencyRecord) *goqu.InsertDataset {
		return d.Insert(city311IdempotencyRecordTable).
			Rows(goqu.Record{
				"id":              res.ID,
				"operation":       res.Operation,
				"key_hash":        res.KeyHash,
				"request_hash":    res.RequestHash,
				"response_status": res.ResponseStatus,
				"response_body":   res.ResponseBody,
				"request_id":      res.RequestID,
				"created_at":      res.CreatedAt,
				"expires_at":      res.ExpiresAt,
			})
	}

	// city311IdempotencyRecordUpsertQuery assembles (insert+on-conflict) query for replacing city311IdempotencyRecords
	//
	// This function is auto-generated
	city311IdempotencyRecordUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311IdempotencyRecord) *goqu.InsertDataset {
		var target = `,id`

		return city311IdempotencyRecordInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"operation":       res.Operation,
						"key_hash":        res.KeyHash,
						"request_hash":    res.RequestHash,
						"response_status": res.ResponseStatus,
						"response_body":   res.ResponseBody,
						"request_id":      res.RequestID,
						"created_at":      res.CreatedAt,
						"expires_at":      res.ExpiresAt,
					},
				),
			)
	}

	// city311IdempotencyRecordUpdateQuery assembles query for updating city311IdempotencyRecords
	//
	// This function is auto-generated
	city311IdempotencyRecordUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311IdempotencyRecord) *goqu.UpdateDataset {
		return d.Update(city311IdempotencyRecordTable).
			Set(goqu.Record{
				"operation":       res.Operation,
				"key_hash":        res.KeyHash,
				"request_hash":    res.RequestHash,
				"response_status": res.ResponseStatus,
				"response_body":   res.ResponseBody,
				"request_id":      res.RequestID,
				"created_at":      res.CreatedAt,
				"expires_at":      res.ExpiresAt,
			}).
			Where(city311IdempotencyRecordPrimaryKeys(res))
	}

	// city311IdempotencyRecordDeleteQuery assembles delete query for removing city311IdempotencyRecords
	//
	// This function is auto-generated
	city311IdempotencyRecordDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311IdempotencyRecordTable).Where(ee...)
	}

	// city311IdempotencyRecordDeleteQuery assembles delete query for removing city311IdempotencyRecords
	//
	// This function is auto-generated
	city311IdempotencyRecordTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311IdempotencyRecordTable)
	}

	// city311IdempotencyRecordPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311IdempotencyRecordPrimaryKeys = func(res *composeType.City311IdempotencyRecord) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311IdentityNotificationTable represents city311IdentityNotifications store table
	//
	// This value is auto-generated
	city311IdentityNotificationTable = goqu.T("compose_city311_identity_notification")

	// city311IdentityNotificationSelectQuery assembles select query for fetching city311IdentityNotifications
	//
	// This function is auto-generated
	city311IdentityNotificationSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"user_id",
			"kind",
			"recipient",
			"delivery_key",
			"payload",
			"status",
			"attempts",
			"last_error",
			"created_at",
			"updated_at",
		).From(city311IdentityNotificationTable)
	}

	// city311IdentityNotificationInsertQuery assembles query inserting city311IdentityNotifications
	//
	// This function is auto-generated
	city311IdentityNotificationInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311IdentityNotification) *goqu.InsertDataset {
		return d.Insert(city311IdentityNotificationTable).
			Rows(goqu.Record{
				"id":           res.ID,
				"user_id":      res.UserID,
				"kind":         res.Kind,
				"recipient":    res.Recipient,
				"delivery_key": res.DeliveryKey,
				"payload":      res.Payload,
				"status":       res.Status,
				"attempts":     res.Attempts,
				"last_error":   res.LastError,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
			})
	}

	// city311IdentityNotificationUpsertQuery assembles (insert+on-conflict) query for replacing city311IdentityNotifications
	//
	// This function is auto-generated
	city311IdentityNotificationUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311IdentityNotification) *goqu.InsertDataset {
		var target = `,id`

		return city311IdentityNotificationInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"user_id":      res.UserID,
						"kind":         res.Kind,
						"recipient":    res.Recipient,
						"delivery_key": res.DeliveryKey,
						"payload":      res.Payload,
						"status":       res.Status,
						"attempts":     res.Attempts,
						"last_error":   res.LastError,
						"created_at":   res.CreatedAt,
						"updated_at":   res.UpdatedAt,
					},
				),
			)
	}

	// city311IdentityNotificationUpdateQuery assembles query for updating city311IdentityNotifications
	//
	// This function is auto-generated
	city311IdentityNotificationUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311IdentityNotification) *goqu.UpdateDataset {
		return d.Update(city311IdentityNotificationTable).
			Set(goqu.Record{
				"user_id":      res.UserID,
				"kind":         res.Kind,
				"recipient":    res.Recipient,
				"delivery_key": res.DeliveryKey,
				"payload":      res.Payload,
				"status":       res.Status,
				"attempts":     res.Attempts,
				"last_error":   res.LastError,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
			}).
			Where(city311IdentityNotificationPrimaryKeys(res))
	}

	// city311IdentityNotificationDeleteQuery assembles delete query for removing city311IdentityNotifications
	//
	// This function is auto-generated
	city311IdentityNotificationDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311IdentityNotificationTable).Where(ee...)
	}

	// city311IdentityNotificationDeleteQuery assembles delete query for removing city311IdentityNotifications
	//
	// This function is auto-generated
	city311IdentityNotificationTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311IdentityNotificationTable)
	}

	// city311IdentityNotificationPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311IdentityNotificationPrimaryKeys = func(res *composeType.City311IdentityNotification) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311IdentitySessionTable represents city311IdentitySessions store table
	//
	// This value is auto-generated
	city311IdentitySessionTable = goqu.T("compose_city311_identity_session")

	// city311IdentitySessionSelectQuery assembles select query for fetching city311IdentitySessions
	//
	// This function is auto-generated
	city311IdentitySessionSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"token_hash",
			"user_id",
			"issued_at",
			"last_seen_at",
			"expires_at",
			"absolute_expires_at",
		).From(city311IdentitySessionTable)
	}

	// city311IdentitySessionInsertQuery assembles query inserting city311IdentitySessions
	//
	// This function is auto-generated
	city311IdentitySessionInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311IdentitySession) *goqu.InsertDataset {
		return d.Insert(city311IdentitySessionTable).
			Rows(goqu.Record{
				"id":                  res.ID,
				"token_hash":          res.TokenHash,
				"user_id":             res.UserID,
				"issued_at":           res.IssuedAt,
				"last_seen_at":        res.LastSeenAt,
				"expires_at":          res.ExpiresAt,
				"absolute_expires_at": res.AbsoluteExpiresAt,
			})
	}

	// city311IdentitySessionUpsertQuery assembles (insert+on-conflict) query for replacing city311IdentitySessions
	//
	// This function is auto-generated
	city311IdentitySessionUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311IdentitySession) *goqu.InsertDataset {
		var target = `,id`

		return city311IdentitySessionInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"token_hash":          res.TokenHash,
						"user_id":             res.UserID,
						"issued_at":           res.IssuedAt,
						"last_seen_at":        res.LastSeenAt,
						"expires_at":          res.ExpiresAt,
						"absolute_expires_at": res.AbsoluteExpiresAt,
					},
				),
			)
	}

	// city311IdentitySessionUpdateQuery assembles query for updating city311IdentitySessions
	//
	// This function is auto-generated
	city311IdentitySessionUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311IdentitySession) *goqu.UpdateDataset {
		return d.Update(city311IdentitySessionTable).
			Set(goqu.Record{
				"token_hash":          res.TokenHash,
				"user_id":             res.UserID,
				"issued_at":           res.IssuedAt,
				"last_seen_at":        res.LastSeenAt,
				"expires_at":          res.ExpiresAt,
				"absolute_expires_at": res.AbsoluteExpiresAt,
			}).
			Where(city311IdentitySessionPrimaryKeys(res))
	}

	// city311IdentitySessionDeleteQuery assembles delete query for removing city311IdentitySessions
	//
	// This function is auto-generated
	city311IdentitySessionDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311IdentitySessionTable).Where(ee...)
	}

	// city311IdentitySessionDeleteQuery assembles delete query for removing city311IdentitySessions
	//
	// This function is auto-generated
	city311IdentitySessionTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311IdentitySessionTable)
	}

	// city311IdentitySessionPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311IdentitySessionPrimaryKeys = func(res *composeType.City311IdentitySession) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311LocalAccountTable represents city311LocalAccounts store table
	//
	// This value is auto-generated
	city311LocalAccountTable = goqu.T("compose_city311_local_account")

	// city311LocalAccountSelectQuery assembles select query for fetching city311LocalAccounts
	//
	// This function is auto-generated
	city311LocalAccountSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"login_identifier",
			"verified_email",
			"preferred_language",
			"created_at",
			"updated_at",
		).From(city311LocalAccountTable)
	}

	// city311LocalAccountInsertQuery assembles query inserting city311LocalAccounts
	//
	// This function is auto-generated
	city311LocalAccountInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311LocalAccount) *goqu.InsertDataset {
		return d.Insert(city311LocalAccountTable).
			Rows(goqu.Record{
				"id":                 res.ID,
				"login_identifier":   res.LoginIdentifier,
				"verified_email":     res.VerifiedEmail,
				"preferred_language": res.PreferredLanguage,
				"created_at":         res.CreatedAt,
				"updated_at":         res.UpdatedAt,
			})
	}

	// city311LocalAccountUpsertQuery assembles (insert+on-conflict) query for replacing city311LocalAccounts
	//
	// This function is auto-generated
	city311LocalAccountUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311LocalAccount) *goqu.InsertDataset {
		var target = `,id`

		return city311LocalAccountInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"login_identifier":   res.LoginIdentifier,
						"verified_email":     res.VerifiedEmail,
						"preferred_language": res.PreferredLanguage,
						"created_at":         res.CreatedAt,
						"updated_at":         res.UpdatedAt,
					},
				),
			)
	}

	// city311LocalAccountUpdateQuery assembles query for updating city311LocalAccounts
	//
	// This function is auto-generated
	city311LocalAccountUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311LocalAccount) *goqu.UpdateDataset {
		return d.Update(city311LocalAccountTable).
			Set(goqu.Record{
				"login_identifier":   res.LoginIdentifier,
				"verified_email":     res.VerifiedEmail,
				"preferred_language": res.PreferredLanguage,
				"created_at":         res.CreatedAt,
				"updated_at":         res.UpdatedAt,
			}).
			Where(city311LocalAccountPrimaryKeys(res))
	}

	// city311LocalAccountDeleteQuery assembles delete query for removing city311LocalAccounts
	//
	// This function is auto-generated
	city311LocalAccountDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311LocalAccountTable).Where(ee...)
	}

	// city311LocalAccountDeleteQuery assembles delete query for removing city311LocalAccounts
	//
	// This function is auto-generated
	city311LocalAccountTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311LocalAccountTable)
	}

	// city311LocalAccountPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311LocalAccountPrimaryKeys = func(res *composeType.City311LocalAccount) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311OperationTable represents city311Operations store table
	//
	// This value is auto-generated
	city311OperationTable = goqu.T("compose_city311_operation")

	// city311OperationSelectQuery assembles select query for fetching city311Operations
	//
	// This function is auto-generated
	city311OperationSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"kind",
			"status",
			"progress",
			"actor_id",
			"result",
			"error",
			"content",
			"content_type",
			"filename",
			"created_at",
			"updated_at",
			"completed_at",
		).From(city311OperationTable)
	}

	// city311OperationInsertQuery assembles query inserting city311Operations
	//
	// This function is auto-generated
	city311OperationInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311Operation) *goqu.InsertDataset {
		return d.Insert(city311OperationTable).
			Rows(goqu.Record{
				"id":           res.ID,
				"kind":         res.Kind,
				"status":       res.Status,
				"progress":     res.Progress,
				"actor_id":     res.ActorID,
				"result":       res.Result,
				"error":        res.Error,
				"content":      res.Content,
				"content_type": res.ContentType,
				"filename":     res.Filename,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"completed_at": res.CompletedAt,
			})
	}

	// city311OperationUpsertQuery assembles (insert+on-conflict) query for replacing city311Operations
	//
	// This function is auto-generated
	city311OperationUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311Operation) *goqu.InsertDataset {
		var target = `,id`

		return city311OperationInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"kind":         res.Kind,
						"status":       res.Status,
						"progress":     res.Progress,
						"actor_id":     res.ActorID,
						"result":       res.Result,
						"error":        res.Error,
						"content":      res.Content,
						"content_type": res.ContentType,
						"filename":     res.Filename,
						"created_at":   res.CreatedAt,
						"updated_at":   res.UpdatedAt,
						"completed_at": res.CompletedAt,
					},
				),
			)
	}

	// city311OperationUpdateQuery assembles query for updating city311Operations
	//
	// This function is auto-generated
	city311OperationUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311Operation) *goqu.UpdateDataset {
		return d.Update(city311OperationTable).
			Set(goqu.Record{
				"kind":         res.Kind,
				"status":       res.Status,
				"progress":     res.Progress,
				"actor_id":     res.ActorID,
				"result":       res.Result,
				"error":        res.Error,
				"content":      res.Content,
				"content_type": res.ContentType,
				"filename":     res.Filename,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"completed_at": res.CompletedAt,
			}).
			Where(city311OperationPrimaryKeys(res))
	}

	// city311OperationDeleteQuery assembles delete query for removing city311Operations
	//
	// This function is auto-generated
	city311OperationDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311OperationTable).Where(ee...)
	}

	// city311OperationDeleteQuery assembles delete query for removing city311Operations
	//
	// This function is auto-generated
	city311OperationTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311OperationTable)
	}

	// city311OperationPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311OperationPrimaryKeys = func(res *composeType.City311Operation) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311PasswordResetTokenTable represents city311PasswordResetTokens store table
	//
	// This value is auto-generated
	city311PasswordResetTokenTable = goqu.T("compose_city311_password_reset_token")

	// city311PasswordResetTokenSelectQuery assembles select query for fetching city311PasswordResetTokens
	//
	// This function is auto-generated
	city311PasswordResetTokenSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"token_hash",
			"user_id",
			"created_at",
			"expires_at",
			"used_at",
		).From(city311PasswordResetTokenTable)
	}

	// city311PasswordResetTokenInsertQuery assembles query inserting city311PasswordResetTokens
	//
	// This function is auto-generated
	city311PasswordResetTokenInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311PasswordResetToken) *goqu.InsertDataset {
		return d.Insert(city311PasswordResetTokenTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"token_hash": res.TokenHash,
				"user_id":    res.UserID,
				"created_at": res.CreatedAt,
				"expires_at": res.ExpiresAt,
				"used_at":    res.UsedAt,
			})
	}

	// city311PasswordResetTokenUpsertQuery assembles (insert+on-conflict) query for replacing city311PasswordResetTokens
	//
	// This function is auto-generated
	city311PasswordResetTokenUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311PasswordResetToken) *goqu.InsertDataset {
		var target = `,id`

		return city311PasswordResetTokenInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"token_hash": res.TokenHash,
						"user_id":    res.UserID,
						"created_at": res.CreatedAt,
						"expires_at": res.ExpiresAt,
						"used_at":    res.UsedAt,
					},
				),
			)
	}

	// city311PasswordResetTokenUpdateQuery assembles query for updating city311PasswordResetTokens
	//
	// This function is auto-generated
	city311PasswordResetTokenUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311PasswordResetToken) *goqu.UpdateDataset {
		return d.Update(city311PasswordResetTokenTable).
			Set(goqu.Record{
				"token_hash": res.TokenHash,
				"user_id":    res.UserID,
				"created_at": res.CreatedAt,
				"expires_at": res.ExpiresAt,
				"used_at":    res.UsedAt,
			}).
			Where(city311PasswordResetTokenPrimaryKeys(res))
	}

	// city311PasswordResetTokenDeleteQuery assembles delete query for removing city311PasswordResetTokens
	//
	// This function is auto-generated
	city311PasswordResetTokenDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311PasswordResetTokenTable).Where(ee...)
	}

	// city311PasswordResetTokenDeleteQuery assembles delete query for removing city311PasswordResetTokens
	//
	// This function is auto-generated
	city311PasswordResetTokenTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311PasswordResetTokenTable)
	}

	// city311PasswordResetTokenPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311PasswordResetTokenPrimaryKeys = func(res *composeType.City311PasswordResetToken) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311PublicHistoryItemTable represents city311PublicHistoryItems store table
	//
	// This value is auto-generated
	city311PublicHistoryItemTable = goqu.T("compose_city311_public_history")

	// city311PublicHistoryItemSelectQuery assembles select query for fetching city311PublicHistoryItems
	//
	// This function is auto-generated
	city311PublicHistoryItemSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"request_id",
			"action",
			"responsible_department",
			"occurred_at",
		).From(city311PublicHistoryItemTable)
	}

	// city311PublicHistoryItemInsertQuery assembles query inserting city311PublicHistoryItems
	//
	// This function is auto-generated
	city311PublicHistoryItemInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311PublicHistoryItem) *goqu.InsertDataset {
		return d.Insert(city311PublicHistoryItemTable).
			Rows(goqu.Record{
				"id":                     res.ID,
				"request_id":             res.RequestID,
				"action":                 res.Action,
				"responsible_department": res.ResponsibleDepartment,
				"occurred_at":            res.OccurredAt,
			})
	}

	// city311PublicHistoryItemUpsertQuery assembles (insert+on-conflict) query for replacing city311PublicHistoryItems
	//
	// This function is auto-generated
	city311PublicHistoryItemUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311PublicHistoryItem) *goqu.InsertDataset {
		var target = `,id`

		return city311PublicHistoryItemInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"request_id":             res.RequestID,
						"action":                 res.Action,
						"responsible_department": res.ResponsibleDepartment,
						"occurred_at":            res.OccurredAt,
					},
				),
			)
	}

	// city311PublicHistoryItemUpdateQuery assembles query for updating city311PublicHistoryItems
	//
	// This function is auto-generated
	city311PublicHistoryItemUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311PublicHistoryItem) *goqu.UpdateDataset {
		return d.Update(city311PublicHistoryItemTable).
			Set(goqu.Record{
				"request_id":             res.RequestID,
				"action":                 res.Action,
				"responsible_department": res.ResponsibleDepartment,
				"occurred_at":            res.OccurredAt,
			}).
			Where(city311PublicHistoryItemPrimaryKeys(res))
	}

	// city311PublicHistoryItemDeleteQuery assembles delete query for removing city311PublicHistoryItems
	//
	// This function is auto-generated
	city311PublicHistoryItemDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311PublicHistoryItemTable).Where(ee...)
	}

	// city311PublicHistoryItemDeleteQuery assembles delete query for removing city311PublicHistoryItems
	//
	// This function is auto-generated
	city311PublicHistoryItemTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311PublicHistoryItemTable)
	}

	// city311PublicHistoryItemPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311PublicHistoryItemPrimaryKeys = func(res *composeType.City311PublicHistoryItem) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311ReopenRequestTable represents city311ReopenRequests store table
	//
	// This value is auto-generated
	city311ReopenRequestTable = goqu.T("compose_city311_reopen_request")

	// city311ReopenRequestSelectQuery assembles select query for fetching city311ReopenRequests
	//
	// This function is auto-generated
	city311ReopenRequestSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"request_id",
			"requested_by",
			"request_reason",
			"status",
			"requested_at",
			"approved_by",
			"approval_reason",
			"approved_at",
		).From(city311ReopenRequestTable)
	}

	// city311ReopenRequestInsertQuery assembles query inserting city311ReopenRequests
	//
	// This function is auto-generated
	city311ReopenRequestInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311ReopenRequest) *goqu.InsertDataset {
		return d.Insert(city311ReopenRequestTable).
			Rows(goqu.Record{
				"id":              res.ID,
				"request_id":      res.RequestID,
				"requested_by":    res.RequestedBy,
				"request_reason":  res.RequestReason,
				"status":          res.Status,
				"requested_at":    res.RequestedAt,
				"approved_by":     res.ApprovedBy,
				"approval_reason": res.ApprovalReason,
				"approved_at":     res.ApprovedAt,
			})
	}

	// city311ReopenRequestUpsertQuery assembles (insert+on-conflict) query for replacing city311ReopenRequests
	//
	// This function is auto-generated
	city311ReopenRequestUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311ReopenRequest) *goqu.InsertDataset {
		var target = `,id`

		return city311ReopenRequestInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"request_id":      res.RequestID,
						"requested_by":    res.RequestedBy,
						"request_reason":  res.RequestReason,
						"status":          res.Status,
						"requested_at":    res.RequestedAt,
						"approved_by":     res.ApprovedBy,
						"approval_reason": res.ApprovalReason,
						"approved_at":     res.ApprovedAt,
					},
				),
			)
	}

	// city311ReopenRequestUpdateQuery assembles query for updating city311ReopenRequests
	//
	// This function is auto-generated
	city311ReopenRequestUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311ReopenRequest) *goqu.UpdateDataset {
		return d.Update(city311ReopenRequestTable).
			Set(goqu.Record{
				"request_id":      res.RequestID,
				"requested_by":    res.RequestedBy,
				"request_reason":  res.RequestReason,
				"status":          res.Status,
				"requested_at":    res.RequestedAt,
				"approved_by":     res.ApprovedBy,
				"approval_reason": res.ApprovalReason,
				"approved_at":     res.ApprovedAt,
			}).
			Where(city311ReopenRequestPrimaryKeys(res))
	}

	// city311ReopenRequestDeleteQuery assembles delete query for removing city311ReopenRequests
	//
	// This function is auto-generated
	city311ReopenRequestDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311ReopenRequestTable).Where(ee...)
	}

	// city311ReopenRequestDeleteQuery assembles delete query for removing city311ReopenRequests
	//
	// This function is auto-generated
	city311ReopenRequestTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311ReopenRequestTable)
	}

	// city311ReopenRequestPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311ReopenRequestPrimaryKeys = func(res *composeType.City311ReopenRequest) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311RequestAttachmentTable represents city311RequestAttachments store table
	//
	// This value is auto-generated
	city311RequestAttachmentTable = goqu.T("compose_city311_request_attachment")

	// city311RequestAttachmentSelectQuery assembles select query for fetching city311RequestAttachments
	//
	// This function is auto-generated
	city311RequestAttachmentSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"request_id",
			"filename",
			"media_type",
			"size",
			"content",
			"created_at",
		).From(city311RequestAttachmentTable)
	}

	// city311RequestAttachmentInsertQuery assembles query inserting city311RequestAttachments
	//
	// This function is auto-generated
	city311RequestAttachmentInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestAttachment) *goqu.InsertDataset {
		return d.Insert(city311RequestAttachmentTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"request_id": res.RequestID,
				"filename":   res.Filename,
				"media_type": res.MediaType,
				"size":       res.Size,
				"content":    res.Content,
				"created_at": res.CreatedAt,
			})
	}

	// city311RequestAttachmentUpsertQuery assembles (insert+on-conflict) query for replacing city311RequestAttachments
	//
	// This function is auto-generated
	city311RequestAttachmentUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestAttachment) *goqu.InsertDataset {
		var target = `,id`

		return city311RequestAttachmentInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"request_id": res.RequestID,
						"filename":   res.Filename,
						"media_type": res.MediaType,
						"size":       res.Size,
						"content":    res.Content,
						"created_at": res.CreatedAt,
					},
				),
			)
	}

	// city311RequestAttachmentUpdateQuery assembles query for updating city311RequestAttachments
	//
	// This function is auto-generated
	city311RequestAttachmentUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestAttachment) *goqu.UpdateDataset {
		return d.Update(city311RequestAttachmentTable).
			Set(goqu.Record{
				"request_id": res.RequestID,
				"filename":   res.Filename,
				"media_type": res.MediaType,
				"size":       res.Size,
				"content":    res.Content,
				"created_at": res.CreatedAt,
			}).
			Where(city311RequestAttachmentPrimaryKeys(res))
	}

	// city311RequestAttachmentDeleteQuery assembles delete query for removing city311RequestAttachments
	//
	// This function is auto-generated
	city311RequestAttachmentDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311RequestAttachmentTable).Where(ee...)
	}

	// city311RequestAttachmentDeleteQuery assembles delete query for removing city311RequestAttachments
	//
	// This function is auto-generated
	city311RequestAttachmentTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311RequestAttachmentTable)
	}

	// city311RequestAttachmentPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311RequestAttachmentPrimaryKeys = func(res *composeType.City311RequestAttachment) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311RequestConstituentLinkTable represents city311RequestConstituentLinks store table
	//
	// This value is auto-generated
	city311RequestConstituentLinkTable = goqu.T("compose_city311_request_constituent")

	// city311RequestConstituentLinkSelectQuery assembles select query for fetching city311RequestConstituentLinks
	//
	// This function is auto-generated
	city311RequestConstituentLinkSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"request_id",
			"constituent_id",
			"relationship_type",
			"portal_visible",
			"notify_status",
			"created_at",
			"updated_at",
		).From(city311RequestConstituentLinkTable)
	}

	// city311RequestConstituentLinkInsertQuery assembles query inserting city311RequestConstituentLinks
	//
	// This function is auto-generated
	city311RequestConstituentLinkInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestConstituent) *goqu.InsertDataset {
		return d.Insert(city311RequestConstituentLinkTable).
			Rows(goqu.Record{
				"id":                res.ID,
				"request_id":        res.RequestID,
				"constituent_id":    res.ConstituentID,
				"relationship_type": res.RelationshipType,
				"portal_visible":    res.PortalVisible,
				"notify_status":     res.NotifyStatus,
				"created_at":        res.CreatedAt,
				"updated_at":        res.UpdatedAt,
			})
	}

	// city311RequestConstituentLinkUpsertQuery assembles (insert+on-conflict) query for replacing city311RequestConstituentLinks
	//
	// This function is auto-generated
	city311RequestConstituentLinkUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestConstituent) *goqu.InsertDataset {
		var target = `,id`

		return city311RequestConstituentLinkInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"request_id":        res.RequestID,
						"constituent_id":    res.ConstituentID,
						"relationship_type": res.RelationshipType,
						"portal_visible":    res.PortalVisible,
						"notify_status":     res.NotifyStatus,
						"created_at":        res.CreatedAt,
						"updated_at":        res.UpdatedAt,
					},
				),
			)
	}

	// city311RequestConstituentLinkUpdateQuery assembles query for updating city311RequestConstituentLinks
	//
	// This function is auto-generated
	city311RequestConstituentLinkUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestConstituent) *goqu.UpdateDataset {
		return d.Update(city311RequestConstituentLinkTable).
			Set(goqu.Record{
				"request_id":        res.RequestID,
				"constituent_id":    res.ConstituentID,
				"relationship_type": res.RelationshipType,
				"portal_visible":    res.PortalVisible,
				"notify_status":     res.NotifyStatus,
				"created_at":        res.CreatedAt,
				"updated_at":        res.UpdatedAt,
			}).
			Where(city311RequestConstituentLinkPrimaryKeys(res))
	}

	// city311RequestConstituentLinkDeleteQuery assembles delete query for removing city311RequestConstituentLinks
	//
	// This function is auto-generated
	city311RequestConstituentLinkDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311RequestConstituentLinkTable).Where(ee...)
	}

	// city311RequestConstituentLinkDeleteQuery assembles delete query for removing city311RequestConstituentLinks
	//
	// This function is auto-generated
	city311RequestConstituentLinkTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311RequestConstituentLinkTable)
	}

	// city311RequestConstituentLinkPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311RequestConstituentLinkPrimaryKeys = func(res *composeType.City311RequestConstituent) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311RequestNoteTable represents city311RequestNotes store table
	//
	// This value is auto-generated
	city311RequestNoteTable = goqu.T("compose_city311_request_note")

	// city311RequestNoteSelectQuery assembles select query for fetching city311RequestNotes
	//
	// This function is auto-generated
	city311RequestNoteSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"request_id",
			"author_type",
			"author_id",
			"author_constituent_id",
			"body",
			"portal_visible",
			"created_at",
		).From(city311RequestNoteTable)
	}

	// city311RequestNoteInsertQuery assembles query inserting city311RequestNotes
	//
	// This function is auto-generated
	city311RequestNoteInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestNote) *goqu.InsertDataset {
		return d.Insert(city311RequestNoteTable).
			Rows(goqu.Record{
				"id":                    res.ID,
				"request_id":            res.RequestID,
				"author_type":           res.AuthorType,
				"author_id":             res.AuthorID,
				"author_constituent_id": res.AuthorConstituentID,
				"body":                  res.Body,
				"portal_visible":        res.PortalVisible,
				"created_at":            res.CreatedAt,
			})
	}

	// city311RequestNoteUpsertQuery assembles (insert+on-conflict) query for replacing city311RequestNotes
	//
	// This function is auto-generated
	city311RequestNoteUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestNote) *goqu.InsertDataset {
		var target = `,id`

		return city311RequestNoteInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"request_id":            res.RequestID,
						"author_type":           res.AuthorType,
						"author_id":             res.AuthorID,
						"author_constituent_id": res.AuthorConstituentID,
						"body":                  res.Body,
						"portal_visible":        res.PortalVisible,
						"created_at":            res.CreatedAt,
					},
				),
			)
	}

	// city311RequestNoteUpdateQuery assembles query for updating city311RequestNotes
	//
	// This function is auto-generated
	city311RequestNoteUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestNote) *goqu.UpdateDataset {
		return d.Update(city311RequestNoteTable).
			Set(goqu.Record{
				"request_id":            res.RequestID,
				"author_type":           res.AuthorType,
				"author_id":             res.AuthorID,
				"author_constituent_id": res.AuthorConstituentID,
				"body":                  res.Body,
				"portal_visible":        res.PortalVisible,
				"created_at":            res.CreatedAt,
			}).
			Where(city311RequestNotePrimaryKeys(res))
	}

	// city311RequestNoteDeleteQuery assembles delete query for removing city311RequestNotes
	//
	// This function is auto-generated
	city311RequestNoteDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311RequestNoteTable).Where(ee...)
	}

	// city311RequestNoteDeleteQuery assembles delete query for removing city311RequestNotes
	//
	// This function is auto-generated
	city311RequestNoteTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311RequestNoteTable)
	}

	// city311RequestNotePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311RequestNotePrimaryKeys = func(res *composeType.City311RequestNote) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311RequestSequenceTable represents city311RequestSequences store table
	//
	// This value is auto-generated
	city311RequestSequenceTable = goqu.T("compose_city311_request_sequence")

	// city311RequestSequenceSelectQuery assembles select query for fetching city311RequestSequences
	//
	// This function is auto-generated
	city311RequestSequenceSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"next_number",
		).From(city311RequestSequenceTable)
	}

	// city311RequestSequenceInsertQuery assembles query inserting city311RequestSequences
	//
	// This function is auto-generated
	city311RequestSequenceInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestSequence) *goqu.InsertDataset {
		return d.Insert(city311RequestSequenceTable).
			Rows(goqu.Record{
				"id":          res.ID,
				"next_number": res.NextNumber,
			})
	}

	// city311RequestSequenceUpsertQuery assembles (insert+on-conflict) query for replacing city311RequestSequences
	//
	// This function is auto-generated
	city311RequestSequenceUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestSequence) *goqu.InsertDataset {
		var target = `,id`

		return city311RequestSequenceInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"next_number": res.NextNumber,
					},
				),
			)
	}

	// city311RequestSequenceUpdateQuery assembles query for updating city311RequestSequences
	//
	// This function is auto-generated
	city311RequestSequenceUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311RequestSequence) *goqu.UpdateDataset {
		return d.Update(city311RequestSequenceTable).
			Set(goqu.Record{
				"next_number": res.NextNumber,
			}).
			Where(city311RequestSequencePrimaryKeys(res))
	}

	// city311RequestSequenceDeleteQuery assembles delete query for removing city311RequestSequences
	//
	// This function is auto-generated
	city311RequestSequenceDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311RequestSequenceTable).Where(ee...)
	}

	// city311RequestSequenceDeleteQuery assembles delete query for removing city311RequestSequences
	//
	// This function is auto-generated
	city311RequestSequenceTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311RequestSequenceTable)
	}

	// city311RequestSequencePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311RequestSequencePrimaryKeys = func(res *composeType.City311RequestSequence) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311ServiceRequestTable represents city311ServiceRequests store table
	//
	// This value is auto-generated
	city311ServiceRequestTable = goqu.T("compose_city311_service_request")

	// city311ServiceRequestSelectQuery assembles select query for fetching city311ServiceRequests
	//
	// This function is auto-generated
	city311ServiceRequestSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"request_number",
			"summary",
			"description",
			"service_type",
			"owning_department",
			"council_district",
			"source_channel",
			"origin_class",
			"status",
			"primary_requester",
			"location",
			"custom_fields",
			"external_work_order",
			"primary_assignee_id",
			"collaborator_ids",
			"duplicate_group_id",
			"scope_department",
			"scope_districts",
			"version",
			"created_at",
			"updated_at",
		).From(city311ServiceRequestTable)
	}

	// city311ServiceRequestInsertQuery assembles query inserting city311ServiceRequests
	//
	// This function is auto-generated
	city311ServiceRequestInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311ServiceRequest) *goqu.InsertDataset {
		return d.Insert(city311ServiceRequestTable).
			Rows(goqu.Record{
				"id":                  res.ID,
				"request_number":      res.RequestNumber,
				"summary":             res.Summary,
				"description":         res.Description,
				"service_type":        res.ServiceType,
				"owning_department":   res.OwningDepartment,
				"council_district":    res.CouncilDistrict,
				"source_channel":      res.SourceChannel,
				"origin_class":        res.OriginClass,
				"status":              res.Status,
				"primary_requester":   res.PrimaryRequester,
				"location":            res.Location,
				"custom_fields":       res.CustomFields,
				"external_work_order": res.ExternalWorkOrder,
				"primary_assignee_id": res.PrimaryAssigneeID,
				"collaborator_ids":    res.CollaboratorIDs,
				"duplicate_group_id":  res.DuplicateGroupID,
				"scope_department":    res.ScopeDepartment,
				"scope_districts":     res.ScopeDistricts,
				"version":             res.Version,
				"created_at":          res.CreatedAt,
				"updated_at":          res.UpdatedAt,
			})
	}

	// city311ServiceRequestUpsertQuery assembles (insert+on-conflict) query for replacing city311ServiceRequests
	//
	// This function is auto-generated
	city311ServiceRequestUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311ServiceRequest) *goqu.InsertDataset {
		var target = `,id`

		return city311ServiceRequestInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"request_number":      res.RequestNumber,
						"summary":             res.Summary,
						"description":         res.Description,
						"service_type":        res.ServiceType,
						"owning_department":   res.OwningDepartment,
						"council_district":    res.CouncilDistrict,
						"source_channel":      res.SourceChannel,
						"origin_class":        res.OriginClass,
						"status":              res.Status,
						"primary_requester":   res.PrimaryRequester,
						"location":            res.Location,
						"custom_fields":       res.CustomFields,
						"external_work_order": res.ExternalWorkOrder,
						"primary_assignee_id": res.PrimaryAssigneeID,
						"collaborator_ids":    res.CollaboratorIDs,
						"duplicate_group_id":  res.DuplicateGroupID,
						"scope_department":    res.ScopeDepartment,
						"scope_districts":     res.ScopeDistricts,
						"version":             res.Version,
						"created_at":          res.CreatedAt,
						"updated_at":          res.UpdatedAt,
					},
				),
			)
	}

	// city311ServiceRequestUpdateQuery assembles query for updating city311ServiceRequests
	//
	// This function is auto-generated
	city311ServiceRequestUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311ServiceRequest) *goqu.UpdateDataset {
		return d.Update(city311ServiceRequestTable).
			Set(goqu.Record{
				"request_number":      res.RequestNumber,
				"summary":             res.Summary,
				"description":         res.Description,
				"service_type":        res.ServiceType,
				"owning_department":   res.OwningDepartment,
				"council_district":    res.CouncilDistrict,
				"source_channel":      res.SourceChannel,
				"origin_class":        res.OriginClass,
				"status":              res.Status,
				"primary_requester":   res.PrimaryRequester,
				"location":            res.Location,
				"custom_fields":       res.CustomFields,
				"external_work_order": res.ExternalWorkOrder,
				"primary_assignee_id": res.PrimaryAssigneeID,
				"collaborator_ids":    res.CollaboratorIDs,
				"duplicate_group_id":  res.DuplicateGroupID,
				"scope_department":    res.ScopeDepartment,
				"scope_districts":     res.ScopeDistricts,
				"version":             res.Version,
				"created_at":          res.CreatedAt,
				"updated_at":          res.UpdatedAt,
			}).
			Where(city311ServiceRequestPrimaryKeys(res))
	}

	// city311ServiceRequestDeleteQuery assembles delete query for removing city311ServiceRequests
	//
	// This function is auto-generated
	city311ServiceRequestDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311ServiceRequestTable).Where(ee...)
	}

	// city311ServiceRequestDeleteQuery assembles delete query for removing city311ServiceRequests
	//
	// This function is auto-generated
	city311ServiceRequestTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311ServiceRequestTable)
	}

	// city311ServiceRequestPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311ServiceRequestPrimaryKeys = func(res *composeType.City311ServiceRequest) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311StagedAttachmentTable represents city311StagedAttachments store table
	//
	// This value is auto-generated
	city311StagedAttachmentTable = goqu.T("compose_city311_staged_attachment")

	// city311StagedAttachmentSelectQuery assembles select query for fetching city311StagedAttachments
	//
	// This function is auto-generated
	city311StagedAttachmentSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"token_hash",
			"owner_id",
			"filename",
			"media_type",
			"content",
			"created_at",
			"expires_at",
		).From(city311StagedAttachmentTable)
	}

	// city311StagedAttachmentInsertQuery assembles query inserting city311StagedAttachments
	//
	// This function is auto-generated
	city311StagedAttachmentInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311StagedAttachment) *goqu.InsertDataset {
		return d.Insert(city311StagedAttachmentTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"token_hash": res.TokenHash,
				"owner_id":   res.OwnerID,
				"filename":   res.Filename,
				"media_type": res.MediaType,
				"content":    res.Content,
				"created_at": res.CreatedAt,
				"expires_at": res.ExpiresAt,
			})
	}

	// city311StagedAttachmentUpsertQuery assembles (insert+on-conflict) query for replacing city311StagedAttachments
	//
	// This function is auto-generated
	city311StagedAttachmentUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311StagedAttachment) *goqu.InsertDataset {
		var target = `,id`

		return city311StagedAttachmentInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"token_hash": res.TokenHash,
						"owner_id":   res.OwnerID,
						"filename":   res.Filename,
						"media_type": res.MediaType,
						"content":    res.Content,
						"created_at": res.CreatedAt,
						"expires_at": res.ExpiresAt,
					},
				),
			)
	}

	// city311StagedAttachmentUpdateQuery assembles query for updating city311StagedAttachments
	//
	// This function is auto-generated
	city311StagedAttachmentUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311StagedAttachment) *goqu.UpdateDataset {
		return d.Update(city311StagedAttachmentTable).
			Set(goqu.Record{
				"token_hash": res.TokenHash,
				"owner_id":   res.OwnerID,
				"filename":   res.Filename,
				"media_type": res.MediaType,
				"content":    res.Content,
				"created_at": res.CreatedAt,
				"expires_at": res.ExpiresAt,
			}).
			Where(city311StagedAttachmentPrimaryKeys(res))
	}

	// city311StagedAttachmentDeleteQuery assembles delete query for removing city311StagedAttachments
	//
	// This function is auto-generated
	city311StagedAttachmentDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311StagedAttachmentTable).Where(ee...)
	}

	// city311StagedAttachmentDeleteQuery assembles delete query for removing city311StagedAttachments
	//
	// This function is auto-generated
	city311StagedAttachmentTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311StagedAttachmentTable)
	}

	// city311StagedAttachmentPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311StagedAttachmentPrimaryKeys = func(res *composeType.City311StagedAttachment) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311WorkflowDefinitionTable represents city311WorkflowDefinitions store table
	//
	// This value is auto-generated
	city311WorkflowDefinitionTable = goqu.T("compose_city311_workflow_definition")

	// city311WorkflowDefinitionSelectQuery assembles select query for fetching city311WorkflowDefinitions
	//
	// This function is auto-generated
	city311WorkflowDefinitionSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"workflow_id",
			"name",
			"trigger",
			"active",
			"definition",
			"version",
			"created_at",
			"updated_at",
		).From(city311WorkflowDefinitionTable)
	}

	// city311WorkflowDefinitionInsertQuery assembles query inserting city311WorkflowDefinitions
	//
	// This function is auto-generated
	city311WorkflowDefinitionInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311WorkflowDefinition) *goqu.InsertDataset {
		return d.Insert(city311WorkflowDefinitionTable).
			Rows(goqu.Record{
				"id":          res.ID,
				"workflow_id": res.WorkflowID,
				"name":        res.Name,
				"trigger":     res.Trigger,
				"active":      res.Active,
				"definition":  res.Definition,
				"version":     res.Version,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
			})
	}

	// city311WorkflowDefinitionUpsertQuery assembles (insert+on-conflict) query for replacing city311WorkflowDefinitions
	//
	// This function is auto-generated
	city311WorkflowDefinitionUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311WorkflowDefinition) *goqu.InsertDataset {
		var target = `,id`

		return city311WorkflowDefinitionInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"workflow_id": res.WorkflowID,
						"name":        res.Name,
						"trigger":     res.Trigger,
						"active":      res.Active,
						"definition":  res.Definition,
						"version":     res.Version,
						"created_at":  res.CreatedAt,
						"updated_at":  res.UpdatedAt,
					},
				),
			)
	}

	// city311WorkflowDefinitionUpdateQuery assembles query for updating city311WorkflowDefinitions
	//
	// This function is auto-generated
	city311WorkflowDefinitionUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311WorkflowDefinition) *goqu.UpdateDataset {
		return d.Update(city311WorkflowDefinitionTable).
			Set(goqu.Record{
				"workflow_id": res.WorkflowID,
				"name":        res.Name,
				"trigger":     res.Trigger,
				"active":      res.Active,
				"definition":  res.Definition,
				"version":     res.Version,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
			}).
			Where(city311WorkflowDefinitionPrimaryKeys(res))
	}

	// city311WorkflowDefinitionDeleteQuery assembles delete query for removing city311WorkflowDefinitions
	//
	// This function is auto-generated
	city311WorkflowDefinitionDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311WorkflowDefinitionTable).Where(ee...)
	}

	// city311WorkflowDefinitionDeleteQuery assembles delete query for removing city311WorkflowDefinitions
	//
	// This function is auto-generated
	city311WorkflowDefinitionTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311WorkflowDefinitionTable)
	}

	// city311WorkflowDefinitionPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311WorkflowDefinitionPrimaryKeys = func(res *composeType.City311WorkflowDefinition) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// city311WorkflowExecutionTable represents city311WorkflowExecutions store table
	//
	// This value is auto-generated
	city311WorkflowExecutionTable = goqu.T("compose_city311_workflow_execution")

	// city311WorkflowExecutionSelectQuery assembles select query for fetching city311WorkflowExecutions
	//
	// This function is auto-generated
	city311WorkflowExecutionSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"execution_id",
			"workflow_id",
			"workflow_version",
			"request_id",
			"trigger",
			"outcome",
			"actions_attempted",
			"succeeded",
			"response_status",
			"error",
			"occurred_at",
		).From(city311WorkflowExecutionTable)
	}

	// city311WorkflowExecutionInsertQuery assembles query inserting city311WorkflowExecutions
	//
	// This function is auto-generated
	city311WorkflowExecutionInsertQuery = func(d goqu.DialectWrapper, res *composeType.City311WorkflowExecution) *goqu.InsertDataset {
		return d.Insert(city311WorkflowExecutionTable).
			Rows(goqu.Record{
				"id":                res.ID,
				"execution_id":      res.ExecutionID,
				"workflow_id":       res.WorkflowID,
				"workflow_version":  res.WorkflowVersion,
				"request_id":        res.RequestID,
				"trigger":           res.Trigger,
				"outcome":           res.Outcome,
				"actions_attempted": res.ActionsAttempted,
				"succeeded":         res.Succeeded,
				"response_status":   res.ResponseStatus,
				"error":             res.Error,
				"occurred_at":       res.OccurredAt,
			})
	}

	// city311WorkflowExecutionUpsertQuery assembles (insert+on-conflict) query for replacing city311WorkflowExecutions
	//
	// This function is auto-generated
	city311WorkflowExecutionUpsertQuery = func(d goqu.DialectWrapper, res *composeType.City311WorkflowExecution) *goqu.InsertDataset {
		var target = `,id`

		return city311WorkflowExecutionInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"execution_id":      res.ExecutionID,
						"workflow_id":       res.WorkflowID,
						"workflow_version":  res.WorkflowVersion,
						"request_id":        res.RequestID,
						"trigger":           res.Trigger,
						"outcome":           res.Outcome,
						"actions_attempted": res.ActionsAttempted,
						"succeeded":         res.Succeeded,
						"response_status":   res.ResponseStatus,
						"error":             res.Error,
						"occurred_at":       res.OccurredAt,
					},
				),
			)
	}

	// city311WorkflowExecutionUpdateQuery assembles query for updating city311WorkflowExecutions
	//
	// This function is auto-generated
	city311WorkflowExecutionUpdateQuery = func(d goqu.DialectWrapper, res *composeType.City311WorkflowExecution) *goqu.UpdateDataset {
		return d.Update(city311WorkflowExecutionTable).
			Set(goqu.Record{
				"execution_id":      res.ExecutionID,
				"workflow_id":       res.WorkflowID,
				"workflow_version":  res.WorkflowVersion,
				"request_id":        res.RequestID,
				"trigger":           res.Trigger,
				"outcome":           res.Outcome,
				"actions_attempted": res.ActionsAttempted,
				"succeeded":         res.Succeeded,
				"response_status":   res.ResponseStatus,
				"error":             res.Error,
				"occurred_at":       res.OccurredAt,
			}).
			Where(city311WorkflowExecutionPrimaryKeys(res))
	}

	// city311WorkflowExecutionDeleteQuery assembles delete query for removing city311WorkflowExecutions
	//
	// This function is auto-generated
	city311WorkflowExecutionDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(city311WorkflowExecutionTable).Where(ee...)
	}

	// city311WorkflowExecutionDeleteQuery assembles delete query for removing city311WorkflowExecutions
	//
	// This function is auto-generated
	city311WorkflowExecutionTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(city311WorkflowExecutionTable)
	}

	// city311WorkflowExecutionPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	city311WorkflowExecutionPrimaryKeys = func(res *composeType.City311WorkflowExecution) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// composeAttachmentTable represents composeAttachments store table
	//
	// This value is auto-generated
	composeAttachmentTable = goqu.T("compose_attachment")

	// composeAttachmentSelectQuery assembles select query for fetching composeAttachments
	//
	// This function is auto-generated
	composeAttachmentSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"rel_namespace",
			"rel_owner",
			"kind",
			"url",
			"preview_url",
			"name",
			"meta",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(composeAttachmentTable)
	}

	// composeAttachmentInsertQuery assembles query inserting composeAttachments
	//
	// This function is auto-generated
	composeAttachmentInsertQuery = func(d goqu.DialectWrapper, res *composeType.Attachment) *goqu.InsertDataset {
		return d.Insert(composeAttachmentTable).
			Rows(goqu.Record{
				"id":            res.ID,
				"rel_namespace": res.NamespaceID,
				"rel_owner":     res.OwnerID,
				"kind":          res.Kind,
				"url":           res.Url,
				"preview_url":   res.PreviewUrl,
				"name":          res.Name,
				"meta":          res.Meta,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			})
	}

	// composeAttachmentUpsertQuery assembles (insert+on-conflict) query for replacing composeAttachments
	//
	// This function is auto-generated
	composeAttachmentUpsertQuery = func(d goqu.DialectWrapper, res *composeType.Attachment) *goqu.InsertDataset {
		var target = `,id`

		return composeAttachmentInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"rel_namespace": res.NamespaceID,
						"rel_owner":     res.OwnerID,
						"kind":          res.Kind,
						"url":           res.Url,
						"preview_url":   res.PreviewUrl,
						"name":          res.Name,
						"meta":          res.Meta,
						"created_at":    res.CreatedAt,
						"updated_at":    res.UpdatedAt,
						"deleted_at":    res.DeletedAt,
					},
				),
			)
	}

	// composeAttachmentUpdateQuery assembles query for updating composeAttachments
	//
	// This function is auto-generated
	composeAttachmentUpdateQuery = func(d goqu.DialectWrapper, res *composeType.Attachment) *goqu.UpdateDataset {
		return d.Update(composeAttachmentTable).
			Set(goqu.Record{
				"rel_namespace": res.NamespaceID,
				"rel_owner":     res.OwnerID,
				"kind":          res.Kind,
				"url":           res.Url,
				"preview_url":   res.PreviewUrl,
				"name":          res.Name,
				"meta":          res.Meta,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			}).
			Where(composeAttachmentPrimaryKeys(res))
	}

	// composeAttachmentDeleteQuery assembles delete query for removing composeAttachments
	//
	// This function is auto-generated
	composeAttachmentDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(composeAttachmentTable).Where(ee...)
	}

	// composeAttachmentDeleteQuery assembles delete query for removing composeAttachments
	//
	// This function is auto-generated
	composeAttachmentTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(composeAttachmentTable)
	}

	// composeAttachmentPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	composeAttachmentPrimaryKeys = func(res *composeType.Attachment) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// composeChartTable represents composeCharts store table
	//
	// This value is auto-generated
	composeChartTable = goqu.T("compose_chart")

	// composeChartSelectQuery assembles select query for fetching composeCharts
	//
	// This function is auto-generated
	composeChartSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"handle",
			"rel_namespace",
			"name",
			"config",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(composeChartTable)
	}

	// composeChartInsertQuery assembles query inserting composeCharts
	//
	// This function is auto-generated
	composeChartInsertQuery = func(d goqu.DialectWrapper, res *composeType.Chart) *goqu.InsertDataset {
		return d.Insert(composeChartTable).
			Rows(goqu.Record{
				"id":            res.ID,
				"handle":        res.Handle,
				"rel_namespace": res.NamespaceID,
				"name":          res.Name,
				"config":        res.Config,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			})
	}

	// composeChartUpsertQuery assembles (insert+on-conflict) query for replacing composeCharts
	//
	// This function is auto-generated
	composeChartUpsertQuery = func(d goqu.DialectWrapper, res *composeType.Chart) *goqu.InsertDataset {
		var target = `,id`

		return composeChartInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"handle":        res.Handle,
						"rel_namespace": res.NamespaceID,
						"name":          res.Name,
						"config":        res.Config,
						"created_at":    res.CreatedAt,
						"updated_at":    res.UpdatedAt,
						"deleted_at":    res.DeletedAt,
					},
				),
			)
	}

	// composeChartUpdateQuery assembles query for updating composeCharts
	//
	// This function is auto-generated
	composeChartUpdateQuery = func(d goqu.DialectWrapper, res *composeType.Chart) *goqu.UpdateDataset {
		return d.Update(composeChartTable).
			Set(goqu.Record{
				"handle":        res.Handle,
				"rel_namespace": res.NamespaceID,
				"name":          res.Name,
				"config":        res.Config,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			}).
			Where(composeChartPrimaryKeys(res))
	}

	// composeChartDeleteQuery assembles delete query for removing composeCharts
	//
	// This function is auto-generated
	composeChartDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(composeChartTable).Where(ee...)
	}

	// composeChartDeleteQuery assembles delete query for removing composeCharts
	//
	// This function is auto-generated
	composeChartTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(composeChartTable)
	}

	// composeChartPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	composeChartPrimaryKeys = func(res *composeType.Chart) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// composeModuleTable represents composeModules store table
	//
	// This value is auto-generated
	composeModuleTable = goqu.T("compose_module")

	// composeModuleSelectQuery assembles select query for fetching composeModules
	//
	// This function is auto-generated
	composeModuleSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"rel_namespace",
			"handle",
			"name",
			"meta",
			"config",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(composeModuleTable)
	}

	// composeModuleInsertQuery assembles query inserting composeModules
	//
	// This function is auto-generated
	composeModuleInsertQuery = func(d goqu.DialectWrapper, res *composeType.Module) *goqu.InsertDataset {
		return d.Insert(composeModuleTable).
			Rows(goqu.Record{
				"id":            res.ID,
				"rel_namespace": res.NamespaceID,
				"handle":        res.Handle,
				"name":          res.Name,
				"meta":          res.Meta,
				"config":        res.Config,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			})
	}

	// composeModuleUpsertQuery assembles (insert+on-conflict) query for replacing composeModules
	//
	// This function is auto-generated
	composeModuleUpsertQuery = func(d goqu.DialectWrapper, res *composeType.Module) *goqu.InsertDataset {
		var target = `,id`

		return composeModuleInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"rel_namespace": res.NamespaceID,
						"handle":        res.Handle,
						"name":          res.Name,
						"meta":          res.Meta,
						"config":        res.Config,
						"created_at":    res.CreatedAt,
						"updated_at":    res.UpdatedAt,
						"deleted_at":    res.DeletedAt,
					},
				),
			)
	}

	// composeModuleUpdateQuery assembles query for updating composeModules
	//
	// This function is auto-generated
	composeModuleUpdateQuery = func(d goqu.DialectWrapper, res *composeType.Module) *goqu.UpdateDataset {
		return d.Update(composeModuleTable).
			Set(goqu.Record{
				"rel_namespace": res.NamespaceID,
				"handle":        res.Handle,
				"name":          res.Name,
				"meta":          res.Meta,
				"config":        res.Config,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			}).
			Where(composeModulePrimaryKeys(res))
	}

	// composeModuleDeleteQuery assembles delete query for removing composeModules
	//
	// This function is auto-generated
	composeModuleDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(composeModuleTable).Where(ee...)
	}

	// composeModuleDeleteQuery assembles delete query for removing composeModules
	//
	// This function is auto-generated
	composeModuleTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(composeModuleTable)
	}

	// composeModulePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	composeModulePrimaryKeys = func(res *composeType.Module) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// composeModuleFieldTable represents composeModuleFields store table
	//
	// This value is auto-generated
	composeModuleFieldTable = goqu.T("compose_module_field")

	// composeModuleFieldSelectQuery assembles select query for fetching composeModuleFields
	//
	// This function is auto-generated
	composeModuleFieldSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"rel_module",
			"place",
			"kind",
			"options",
			"name",
			"label",
			"config",
			"is_required",
			"is_multi",
			"default_value",
			"expressions",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(composeModuleFieldTable)
	}

	// composeModuleFieldInsertQuery assembles query inserting composeModuleFields
	//
	// This function is auto-generated
	composeModuleFieldInsertQuery = func(d goqu.DialectWrapper, res *composeType.ModuleField) *goqu.InsertDataset {
		return d.Insert(composeModuleFieldTable).
			Rows(goqu.Record{
				"id":            res.ID,
				"rel_module":    res.ModuleID,
				"place":         res.Place,
				"kind":          res.Kind,
				"options":       res.Options,
				"name":          res.Name,
				"label":         res.Label,
				"config":        res.Config,
				"is_required":   res.Required,
				"is_multi":      res.Multi,
				"default_value": res.DefaultValue,
				"expressions":   res.Expressions,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			})
	}

	// composeModuleFieldUpsertQuery assembles (insert+on-conflict) query for replacing composeModuleFields
	//
	// This function is auto-generated
	composeModuleFieldUpsertQuery = func(d goqu.DialectWrapper, res *composeType.ModuleField) *goqu.InsertDataset {
		var target = `,id`

		return composeModuleFieldInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"rel_module":    res.ModuleID,
						"place":         res.Place,
						"kind":          res.Kind,
						"options":       res.Options,
						"name":          res.Name,
						"label":         res.Label,
						"config":        res.Config,
						"is_required":   res.Required,
						"is_multi":      res.Multi,
						"default_value": res.DefaultValue,
						"expressions":   res.Expressions,
						"created_at":    res.CreatedAt,
						"updated_at":    res.UpdatedAt,
						"deleted_at":    res.DeletedAt,
					},
				),
			)
	}

	// composeModuleFieldUpdateQuery assembles query for updating composeModuleFields
	//
	// This function is auto-generated
	composeModuleFieldUpdateQuery = func(d goqu.DialectWrapper, res *composeType.ModuleField) *goqu.UpdateDataset {
		return d.Update(composeModuleFieldTable).
			Set(goqu.Record{
				"rel_module":    res.ModuleID,
				"place":         res.Place,
				"kind":          res.Kind,
				"options":       res.Options,
				"name":          res.Name,
				"label":         res.Label,
				"config":        res.Config,
				"is_required":   res.Required,
				"is_multi":      res.Multi,
				"default_value": res.DefaultValue,
				"expressions":   res.Expressions,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			}).
			Where(composeModuleFieldPrimaryKeys(res))
	}

	// composeModuleFieldDeleteQuery assembles delete query for removing composeModuleFields
	//
	// This function is auto-generated
	composeModuleFieldDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(composeModuleFieldTable).Where(ee...)
	}

	// composeModuleFieldDeleteQuery assembles delete query for removing composeModuleFields
	//
	// This function is auto-generated
	composeModuleFieldTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(composeModuleFieldTable)
	}

	// composeModuleFieldPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	composeModuleFieldPrimaryKeys = func(res *composeType.ModuleField) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// composeNamespaceTable represents composeNamespaces store table
	//
	// This value is auto-generated
	composeNamespaceTable = goqu.T("compose_namespace")

	// composeNamespaceSelectQuery assembles select query for fetching composeNamespaces
	//
	// This function is auto-generated
	composeNamespaceSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"slug",
			"enabled",
			"meta",
			"name",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(composeNamespaceTable)
	}

	// composeNamespaceInsertQuery assembles query inserting composeNamespaces
	//
	// This function is auto-generated
	composeNamespaceInsertQuery = func(d goqu.DialectWrapper, res *composeType.Namespace) *goqu.InsertDataset {
		return d.Insert(composeNamespaceTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"slug":       res.Slug,
				"enabled":    res.Enabled,
				"meta":       res.Meta,
				"name":       res.Name,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
			})
	}

	// composeNamespaceUpsertQuery assembles (insert+on-conflict) query for replacing composeNamespaces
	//
	// This function is auto-generated
	composeNamespaceUpsertQuery = func(d goqu.DialectWrapper, res *composeType.Namespace) *goqu.InsertDataset {
		var target = `,id`

		return composeNamespaceInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"slug":       res.Slug,
						"enabled":    res.Enabled,
						"meta":       res.Meta,
						"name":       res.Name,
						"created_at": res.CreatedAt,
						"updated_at": res.UpdatedAt,
						"deleted_at": res.DeletedAt,
					},
				),
			)
	}

	// composeNamespaceUpdateQuery assembles query for updating composeNamespaces
	//
	// This function is auto-generated
	composeNamespaceUpdateQuery = func(d goqu.DialectWrapper, res *composeType.Namespace) *goqu.UpdateDataset {
		return d.Update(composeNamespaceTable).
			Set(goqu.Record{
				"slug":       res.Slug,
				"enabled":    res.Enabled,
				"meta":       res.Meta,
				"name":       res.Name,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
			}).
			Where(composeNamespacePrimaryKeys(res))
	}

	// composeNamespaceDeleteQuery assembles delete query for removing composeNamespaces
	//
	// This function is auto-generated
	composeNamespaceDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(composeNamespaceTable).Where(ee...)
	}

	// composeNamespaceDeleteQuery assembles delete query for removing composeNamespaces
	//
	// This function is auto-generated
	composeNamespaceTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(composeNamespaceTable)
	}

	// composeNamespacePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	composeNamespacePrimaryKeys = func(res *composeType.Namespace) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// composePageTable represents composePages store table
	//
	// This value is auto-generated
	composePageTable = goqu.T("compose_page")

	// composePageSelectQuery assembles select query for fetching composePages
	//
	// This function is auto-generated
	composePageSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"title",
			"handle",
			"self_id",
			"rel_module",
			"rel_namespace",
			"meta",
			"config",
			"blocks",
			"visible",
			"weight",
			"description",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(composePageTable)
	}

	// composePageInsertQuery assembles query inserting composePages
	//
	// This function is auto-generated
	composePageInsertQuery = func(d goqu.DialectWrapper, res *composeType.Page) *goqu.InsertDataset {
		return d.Insert(composePageTable).
			Rows(goqu.Record{
				"id":            res.ID,
				"title":         res.Title,
				"handle":        res.Handle,
				"self_id":       res.SelfID,
				"rel_module":    res.ModuleID,
				"rel_namespace": res.NamespaceID,
				"meta":          res.Meta,
				"config":        res.Config,
				"blocks":        res.Blocks,
				"visible":       res.Visible,
				"weight":        res.Weight,
				"description":   res.Description,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			})
	}

	// composePageUpsertQuery assembles (insert+on-conflict) query for replacing composePages
	//
	// This function is auto-generated
	composePageUpsertQuery = func(d goqu.DialectWrapper, res *composeType.Page) *goqu.InsertDataset {
		var target = `,id`

		return composePageInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"title":         res.Title,
						"handle":        res.Handle,
						"self_id":       res.SelfID,
						"rel_module":    res.ModuleID,
						"rel_namespace": res.NamespaceID,
						"meta":          res.Meta,
						"config":        res.Config,
						"blocks":        res.Blocks,
						"visible":       res.Visible,
						"weight":        res.Weight,
						"description":   res.Description,
						"created_at":    res.CreatedAt,
						"updated_at":    res.UpdatedAt,
						"deleted_at":    res.DeletedAt,
					},
				),
			)
	}

	// composePageUpdateQuery assembles query for updating composePages
	//
	// This function is auto-generated
	composePageUpdateQuery = func(d goqu.DialectWrapper, res *composeType.Page) *goqu.UpdateDataset {
		return d.Update(composePageTable).
			Set(goqu.Record{
				"title":         res.Title,
				"handle":        res.Handle,
				"self_id":       res.SelfID,
				"rel_module":    res.ModuleID,
				"rel_namespace": res.NamespaceID,
				"meta":          res.Meta,
				"config":        res.Config,
				"blocks":        res.Blocks,
				"visible":       res.Visible,
				"weight":        res.Weight,
				"description":   res.Description,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			}).
			Where(composePagePrimaryKeys(res))
	}

	// composePageDeleteQuery assembles delete query for removing composePages
	//
	// This function is auto-generated
	composePageDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(composePageTable).Where(ee...)
	}

	// composePageDeleteQuery assembles delete query for removing composePages
	//
	// This function is auto-generated
	composePageTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(composePageTable)
	}

	// composePagePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	composePagePrimaryKeys = func(res *composeType.Page) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// composePageLayoutTable represents composePageLayouts store table
	//
	// This value is auto-generated
	composePageLayoutTable = goqu.T("compose_page_layout")

	// composePageLayoutSelectQuery assembles select query for fetching composePageLayouts
	//
	// This function is auto-generated
	composePageLayoutSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"handle",
			"page_id",
			"parent_id",
			"rel_namespace",
			"weight",
			"meta",
			"config",
			"blocks",
			"owned_by",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(composePageLayoutTable)
	}

	// composePageLayoutInsertQuery assembles query inserting composePageLayouts
	//
	// This function is auto-generated
	composePageLayoutInsertQuery = func(d goqu.DialectWrapper, res *composeType.PageLayout) *goqu.InsertDataset {
		return d.Insert(composePageLayoutTable).
			Rows(goqu.Record{
				"id":            res.ID,
				"handle":        res.Handle,
				"page_id":       res.PageID,
				"parent_id":     res.ParentID,
				"rel_namespace": res.NamespaceID,
				"weight":        res.Weight,
				"meta":          res.Meta,
				"config":        res.Config,
				"blocks":        res.Blocks,
				"owned_by":      res.OwnedBy,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			})
	}

	// composePageLayoutUpsertQuery assembles (insert+on-conflict) query for replacing composePageLayouts
	//
	// This function is auto-generated
	composePageLayoutUpsertQuery = func(d goqu.DialectWrapper, res *composeType.PageLayout) *goqu.InsertDataset {
		var target = `,id`

		return composePageLayoutInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"handle":        res.Handle,
						"page_id":       res.PageID,
						"parent_id":     res.ParentID,
						"rel_namespace": res.NamespaceID,
						"weight":        res.Weight,
						"meta":          res.Meta,
						"config":        res.Config,
						"blocks":        res.Blocks,
						"owned_by":      res.OwnedBy,
						"created_at":    res.CreatedAt,
						"updated_at":    res.UpdatedAt,
						"deleted_at":    res.DeletedAt,
					},
				),
			)
	}

	// composePageLayoutUpdateQuery assembles query for updating composePageLayouts
	//
	// This function is auto-generated
	composePageLayoutUpdateQuery = func(d goqu.DialectWrapper, res *composeType.PageLayout) *goqu.UpdateDataset {
		return d.Update(composePageLayoutTable).
			Set(goqu.Record{
				"handle":        res.Handle,
				"page_id":       res.PageID,
				"parent_id":     res.ParentID,
				"rel_namespace": res.NamespaceID,
				"weight":        res.Weight,
				"meta":          res.Meta,
				"config":        res.Config,
				"blocks":        res.Blocks,
				"owned_by":      res.OwnedBy,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
			}).
			Where(composePageLayoutPrimaryKeys(res))
	}

	// composePageLayoutDeleteQuery assembles delete query for removing composePageLayouts
	//
	// This function is auto-generated
	composePageLayoutDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(composePageLayoutTable).Where(ee...)
	}

	// composePageLayoutDeleteQuery assembles delete query for removing composePageLayouts
	//
	// This function is auto-generated
	composePageLayoutTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(composePageLayoutTable)
	}

	// composePageLayoutPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	composePageLayoutPrimaryKeys = func(res *composeType.PageLayout) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// credentialTable represents credentials store table
	//
	// This value is auto-generated
	credentialTable = goqu.T("credentials")

	// credentialSelectQuery assembles select query for fetching credentials
	//
	// This function is auto-generated
	credentialSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"rel_owner",
			"label",
			"kind",
			"credentials",
			"meta",
			"created_at",
			"updated_at",
			"deleted_at",
			"last_used_at",
			"expires_at",
		).From(credentialTable)
	}

	// credentialInsertQuery assembles query inserting credentials
	//
	// This function is auto-generated
	credentialInsertQuery = func(d goqu.DialectWrapper, res *systemType.Credential) *goqu.InsertDataset {
		return d.Insert(credentialTable).
			Rows(goqu.Record{
				"id":           res.ID,
				"rel_owner":    res.OwnerID,
				"label":        res.Label,
				"kind":         res.Kind,
				"credentials":  res.Credentials,
				"meta":         res.Meta,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"deleted_at":   res.DeletedAt,
				"last_used_at": res.LastUsedAt,
				"expires_at":   res.ExpiresAt,
			})
	}

	// credentialUpsertQuery assembles (insert+on-conflict) query for replacing credentials
	//
	// This function is auto-generated
	credentialUpsertQuery = func(d goqu.DialectWrapper, res *systemType.Credential) *goqu.InsertDataset {
		var target = `,id`

		return credentialInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"rel_owner":    res.OwnerID,
						"label":        res.Label,
						"kind":         res.Kind,
						"credentials":  res.Credentials,
						"meta":         res.Meta,
						"created_at":   res.CreatedAt,
						"updated_at":   res.UpdatedAt,
						"deleted_at":   res.DeletedAt,
						"last_used_at": res.LastUsedAt,
						"expires_at":   res.ExpiresAt,
					},
				),
			)
	}

	// credentialUpdateQuery assembles query for updating credentials
	//
	// This function is auto-generated
	credentialUpdateQuery = func(d goqu.DialectWrapper, res *systemType.Credential) *goqu.UpdateDataset {
		return d.Update(credentialTable).
			Set(goqu.Record{
				"rel_owner":    res.OwnerID,
				"label":        res.Label,
				"kind":         res.Kind,
				"credentials":  res.Credentials,
				"meta":         res.Meta,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"deleted_at":   res.DeletedAt,
				"last_used_at": res.LastUsedAt,
				"expires_at":   res.ExpiresAt,
			}).
			Where(credentialPrimaryKeys(res))
	}

	// credentialDeleteQuery assembles delete query for removing credentials
	//
	// This function is auto-generated
	credentialDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(credentialTable).Where(ee...)
	}

	// credentialDeleteQuery assembles delete query for removing credentials
	//
	// This function is auto-generated
	credentialTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(credentialTable)
	}

	// credentialPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	credentialPrimaryKeys = func(res *systemType.Credential) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// dalConnectionTable represents dalConnections store table
	//
	// This value is auto-generated
	dalConnectionTable = goqu.T("dal_connections")

	// dalConnectionSelectQuery assembles select query for fetching dalConnections
	//
	// This function is auto-generated
	dalConnectionSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"handle",
			"type",
			"config",
			"meta",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(dalConnectionTable)
	}

	// dalConnectionInsertQuery assembles query inserting dalConnections
	//
	// This function is auto-generated
	dalConnectionInsertQuery = func(d goqu.DialectWrapper, res *systemType.DalConnection) *goqu.InsertDataset {
		return d.Insert(dalConnectionTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"handle":     res.Handle,
				"type":       res.Type,
				"config":     res.Config,
				"meta":       res.Meta,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			})
	}

	// dalConnectionUpsertQuery assembles (insert+on-conflict) query for replacing dalConnections
	//
	// This function is auto-generated
	dalConnectionUpsertQuery = func(d goqu.DialectWrapper, res *systemType.DalConnection) *goqu.InsertDataset {
		var target = `,id`

		return dalConnectionInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"handle":     res.Handle,
						"type":       res.Type,
						"config":     res.Config,
						"meta":       res.Meta,
						"created_at": res.CreatedAt,
						"updated_at": res.UpdatedAt,
						"deleted_at": res.DeletedAt,
						"created_by": res.CreatedBy,
						"updated_by": res.UpdatedBy,
						"deleted_by": res.DeletedBy,
					},
				),
			)
	}

	// dalConnectionUpdateQuery assembles query for updating dalConnections
	//
	// This function is auto-generated
	dalConnectionUpdateQuery = func(d goqu.DialectWrapper, res *systemType.DalConnection) *goqu.UpdateDataset {
		return d.Update(dalConnectionTable).
			Set(goqu.Record{
				"handle":     res.Handle,
				"type":       res.Type,
				"config":     res.Config,
				"meta":       res.Meta,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			}).
			Where(dalConnectionPrimaryKeys(res))
	}

	// dalConnectionDeleteQuery assembles delete query for removing dalConnections
	//
	// This function is auto-generated
	dalConnectionDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(dalConnectionTable).Where(ee...)
	}

	// dalConnectionDeleteQuery assembles delete query for removing dalConnections
	//
	// This function is auto-generated
	dalConnectionTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(dalConnectionTable)
	}

	// dalConnectionPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	dalConnectionPrimaryKeys = func(res *systemType.DalConnection) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// dalSchemaAlterationTable represents dalSchemaAlterations store table
	//
	// This value is auto-generated
	dalSchemaAlterationTable = goqu.T("dal_schema_alterations")

	// dalSchemaAlterationSelectQuery assembles select query for fetching dalSchemaAlterations
	//
	// This function is auto-generated
	dalSchemaAlterationSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"batch_id",
			"depends_on",
			"resource",
			"resource_type",
			"connection_id",
			"kind",
			"params",
			"error",
			"created_at",
			"updated_at",
			"deleted_at",
			"completed_at",
			"dismissed_at",
			"created_by",
			"updated_by",
			"deleted_by",
			"completed_by",
			"dismissed_by",
		).From(dalSchemaAlterationTable)
	}

	// dalSchemaAlterationInsertQuery assembles query inserting dalSchemaAlterations
	//
	// This function is auto-generated
	dalSchemaAlterationInsertQuery = func(d goqu.DialectWrapper, res *systemType.DalSchemaAlteration) *goqu.InsertDataset {
		return d.Insert(dalSchemaAlterationTable).
			Rows(goqu.Record{
				"id":            res.ID,
				"batch_id":      res.BatchID,
				"depends_on":    res.DependsOn,
				"resource":      res.Resource,
				"resource_type": res.ResourceType,
				"connection_id": res.ConnectionID,
				"kind":          res.Kind,
				"params":        res.Params,
				"error":         res.Error,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
				"completed_at":  res.CompletedAt,
				"dismissed_at":  res.DismissedAt,
				"created_by":    res.CreatedBy,
				"updated_by":    res.UpdatedBy,
				"deleted_by":    res.DeletedBy,
				"completed_by":  res.CompletedBy,
				"dismissed_by":  res.DismissedBy,
			})
	}

	// dalSchemaAlterationUpsertQuery assembles (insert+on-conflict) query for replacing dalSchemaAlterations
	//
	// This function is auto-generated
	dalSchemaAlterationUpsertQuery = func(d goqu.DialectWrapper, res *systemType.DalSchemaAlteration) *goqu.InsertDataset {
		var target = `,id`

		return dalSchemaAlterationInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"batch_id":      res.BatchID,
						"depends_on":    res.DependsOn,
						"resource":      res.Resource,
						"resource_type": res.ResourceType,
						"connection_id": res.ConnectionID,
						"kind":          res.Kind,
						"params":        res.Params,
						"error":         res.Error,
						"created_at":    res.CreatedAt,
						"updated_at":    res.UpdatedAt,
						"deleted_at":    res.DeletedAt,
						"completed_at":  res.CompletedAt,
						"dismissed_at":  res.DismissedAt,
						"created_by":    res.CreatedBy,
						"updated_by":    res.UpdatedBy,
						"deleted_by":    res.DeletedBy,
						"completed_by":  res.CompletedBy,
						"dismissed_by":  res.DismissedBy,
					},
				),
			)
	}

	// dalSchemaAlterationUpdateQuery assembles query for updating dalSchemaAlterations
	//
	// This function is auto-generated
	dalSchemaAlterationUpdateQuery = func(d goqu.DialectWrapper, res *systemType.DalSchemaAlteration) *goqu.UpdateDataset {
		return d.Update(dalSchemaAlterationTable).
			Set(goqu.Record{
				"batch_id":      res.BatchID,
				"depends_on":    res.DependsOn,
				"resource":      res.Resource,
				"resource_type": res.ResourceType,
				"connection_id": res.ConnectionID,
				"kind":          res.Kind,
				"params":        res.Params,
				"error":         res.Error,
				"created_at":    res.CreatedAt,
				"updated_at":    res.UpdatedAt,
				"deleted_at":    res.DeletedAt,
				"completed_at":  res.CompletedAt,
				"dismissed_at":  res.DismissedAt,
				"created_by":    res.CreatedBy,
				"updated_by":    res.UpdatedBy,
				"deleted_by":    res.DeletedBy,
				"completed_by":  res.CompletedBy,
				"dismissed_by":  res.DismissedBy,
			}).
			Where(dalSchemaAlterationPrimaryKeys(res))
	}

	// dalSchemaAlterationDeleteQuery assembles delete query for removing dalSchemaAlterations
	//
	// This function is auto-generated
	dalSchemaAlterationDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(dalSchemaAlterationTable).Where(ee...)
	}

	// dalSchemaAlterationDeleteQuery assembles delete query for removing dalSchemaAlterations
	//
	// This function is auto-generated
	dalSchemaAlterationTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(dalSchemaAlterationTable)
	}

	// dalSchemaAlterationPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	dalSchemaAlterationPrimaryKeys = func(res *systemType.DalSchemaAlteration) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// dalSensitivityLevelTable represents dalSensitivityLevels store table
	//
	// This value is auto-generated
	dalSensitivityLevelTable = goqu.T("dal_sensitivity_levels")

	// dalSensitivityLevelSelectQuery assembles select query for fetching dalSensitivityLevels
	//
	// This function is auto-generated
	dalSensitivityLevelSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"handle",
			"level",
			"meta",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(dalSensitivityLevelTable)
	}

	// dalSensitivityLevelInsertQuery assembles query inserting dalSensitivityLevels
	//
	// This function is auto-generated
	dalSensitivityLevelInsertQuery = func(d goqu.DialectWrapper, res *systemType.DalSensitivityLevel) *goqu.InsertDataset {
		return d.Insert(dalSensitivityLevelTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"handle":     res.Handle,
				"level":      res.Level,
				"meta":       res.Meta,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			})
	}

	// dalSensitivityLevelUpsertQuery assembles (insert+on-conflict) query for replacing dalSensitivityLevels
	//
	// This function is auto-generated
	dalSensitivityLevelUpsertQuery = func(d goqu.DialectWrapper, res *systemType.DalSensitivityLevel) *goqu.InsertDataset {
		var target = `,id`

		return dalSensitivityLevelInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"handle":     res.Handle,
						"level":      res.Level,
						"meta":       res.Meta,
						"created_at": res.CreatedAt,
						"updated_at": res.UpdatedAt,
						"deleted_at": res.DeletedAt,
						"created_by": res.CreatedBy,
						"updated_by": res.UpdatedBy,
						"deleted_by": res.DeletedBy,
					},
				),
			)
	}

	// dalSensitivityLevelUpdateQuery assembles query for updating dalSensitivityLevels
	//
	// This function is auto-generated
	dalSensitivityLevelUpdateQuery = func(d goqu.DialectWrapper, res *systemType.DalSensitivityLevel) *goqu.UpdateDataset {
		return d.Update(dalSensitivityLevelTable).
			Set(goqu.Record{
				"handle":     res.Handle,
				"level":      res.Level,
				"meta":       res.Meta,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			}).
			Where(dalSensitivityLevelPrimaryKeys(res))
	}

	// dalSensitivityLevelDeleteQuery assembles delete query for removing dalSensitivityLevels
	//
	// This function is auto-generated
	dalSensitivityLevelDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(dalSensitivityLevelTable).Where(ee...)
	}

	// dalSensitivityLevelDeleteQuery assembles delete query for removing dalSensitivityLevels
	//
	// This function is auto-generated
	dalSensitivityLevelTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(dalSensitivityLevelTable)
	}

	// dalSensitivityLevelPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	dalSensitivityLevelPrimaryKeys = func(res *systemType.DalSensitivityLevel) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// dataPrivacyRequestTable represents dataPrivacyRequests store table
	//
	// This value is auto-generated
	dataPrivacyRequestTable = goqu.T("data_privacy_requests")

	// dataPrivacyRequestSelectQuery assembles select query for fetching dataPrivacyRequests
	//
	// This function is auto-generated
	dataPrivacyRequestSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"kind",
			"status",
			"payload",
			"requested_at",
			"requested_by",
			"completed_at",
			"completed_by",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(dataPrivacyRequestTable)
	}

	// dataPrivacyRequestInsertQuery assembles query inserting dataPrivacyRequests
	//
	// This function is auto-generated
	dataPrivacyRequestInsertQuery = func(d goqu.DialectWrapper, res *systemType.DataPrivacyRequest) *goqu.InsertDataset {
		return d.Insert(dataPrivacyRequestTable).
			Rows(goqu.Record{
				"id":           res.ID,
				"kind":         res.Kind,
				"status":       res.Status,
				"payload":      res.Payload,
				"requested_at": res.RequestedAt,
				"requested_by": res.RequestedBy,
				"completed_at": res.CompletedAt,
				"completed_by": res.CompletedBy,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"deleted_at":   res.DeletedAt,
				"created_by":   res.CreatedBy,
				"updated_by":   res.UpdatedBy,
				"deleted_by":   res.DeletedBy,
			})
	}

	// dataPrivacyRequestUpsertQuery assembles (insert+on-conflict) query for replacing dataPrivacyRequests
	//
	// This function is auto-generated
	dataPrivacyRequestUpsertQuery = func(d goqu.DialectWrapper, res *systemType.DataPrivacyRequest) *goqu.InsertDataset {
		var target = `,id`

		return dataPrivacyRequestInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"kind":         res.Kind,
						"status":       res.Status,
						"payload":      res.Payload,
						"requested_at": res.RequestedAt,
						"requested_by": res.RequestedBy,
						"completed_at": res.CompletedAt,
						"completed_by": res.CompletedBy,
						"created_at":   res.CreatedAt,
						"updated_at":   res.UpdatedAt,
						"deleted_at":   res.DeletedAt,
						"created_by":   res.CreatedBy,
						"updated_by":   res.UpdatedBy,
						"deleted_by":   res.DeletedBy,
					},
				),
			)
	}

	// dataPrivacyRequestUpdateQuery assembles query for updating dataPrivacyRequests
	//
	// This function is auto-generated
	dataPrivacyRequestUpdateQuery = func(d goqu.DialectWrapper, res *systemType.DataPrivacyRequest) *goqu.UpdateDataset {
		return d.Update(dataPrivacyRequestTable).
			Set(goqu.Record{
				"kind":         res.Kind,
				"status":       res.Status,
				"payload":      res.Payload,
				"requested_at": res.RequestedAt,
				"requested_by": res.RequestedBy,
				"completed_at": res.CompletedAt,
				"completed_by": res.CompletedBy,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"deleted_at":   res.DeletedAt,
				"created_by":   res.CreatedBy,
				"updated_by":   res.UpdatedBy,
				"deleted_by":   res.DeletedBy,
			}).
			Where(dataPrivacyRequestPrimaryKeys(res))
	}

	// dataPrivacyRequestDeleteQuery assembles delete query for removing dataPrivacyRequests
	//
	// This function is auto-generated
	dataPrivacyRequestDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(dataPrivacyRequestTable).Where(ee...)
	}

	// dataPrivacyRequestDeleteQuery assembles delete query for removing dataPrivacyRequests
	//
	// This function is auto-generated
	dataPrivacyRequestTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(dataPrivacyRequestTable)
	}

	// dataPrivacyRequestPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	dataPrivacyRequestPrimaryKeys = func(res *systemType.DataPrivacyRequest) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// dataPrivacyRequestCommentTable represents dataPrivacyRequestComments store table
	//
	// This value is auto-generated
	dataPrivacyRequestCommentTable = goqu.T("data_privacy_request_comments")

	// dataPrivacyRequestCommentSelectQuery assembles select query for fetching dataPrivacyRequestComments
	//
	// This function is auto-generated
	dataPrivacyRequestCommentSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"rel_request",
			"comment",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(dataPrivacyRequestCommentTable)
	}

	// dataPrivacyRequestCommentInsertQuery assembles query inserting dataPrivacyRequestComments
	//
	// This function is auto-generated
	dataPrivacyRequestCommentInsertQuery = func(d goqu.DialectWrapper, res *systemType.DataPrivacyRequestComment) *goqu.InsertDataset {
		return d.Insert(dataPrivacyRequestCommentTable).
			Rows(goqu.Record{
				"id":          res.ID,
				"rel_request": res.RequestID,
				"comment":     res.Comment,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
				"deleted_at":  res.DeletedAt,
				"created_by":  res.CreatedBy,
				"updated_by":  res.UpdatedBy,
				"deleted_by":  res.DeletedBy,
			})
	}

	// dataPrivacyRequestCommentUpsertQuery assembles (insert+on-conflict) query for replacing dataPrivacyRequestComments
	//
	// This function is auto-generated
	dataPrivacyRequestCommentUpsertQuery = func(d goqu.DialectWrapper, res *systemType.DataPrivacyRequestComment) *goqu.InsertDataset {
		var target = `,id`

		return dataPrivacyRequestCommentInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"rel_request": res.RequestID,
						"comment":     res.Comment,
						"created_at":  res.CreatedAt,
						"updated_at":  res.UpdatedAt,
						"deleted_at":  res.DeletedAt,
						"created_by":  res.CreatedBy,
						"updated_by":  res.UpdatedBy,
						"deleted_by":  res.DeletedBy,
					},
				),
			)
	}

	// dataPrivacyRequestCommentUpdateQuery assembles query for updating dataPrivacyRequestComments
	//
	// This function is auto-generated
	dataPrivacyRequestCommentUpdateQuery = func(d goqu.DialectWrapper, res *systemType.DataPrivacyRequestComment) *goqu.UpdateDataset {
		return d.Update(dataPrivacyRequestCommentTable).
			Set(goqu.Record{
				"rel_request": res.RequestID,
				"comment":     res.Comment,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
				"deleted_at":  res.DeletedAt,
				"created_by":  res.CreatedBy,
				"updated_by":  res.UpdatedBy,
				"deleted_by":  res.DeletedBy,
			}).
			Where(dataPrivacyRequestCommentPrimaryKeys(res))
	}

	// dataPrivacyRequestCommentDeleteQuery assembles delete query for removing dataPrivacyRequestComments
	//
	// This function is auto-generated
	dataPrivacyRequestCommentDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(dataPrivacyRequestCommentTable).Where(ee...)
	}

	// dataPrivacyRequestCommentDeleteQuery assembles delete query for removing dataPrivacyRequestComments
	//
	// This function is auto-generated
	dataPrivacyRequestCommentTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(dataPrivacyRequestCommentTable)
	}

	// dataPrivacyRequestCommentPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	dataPrivacyRequestCommentPrimaryKeys = func(res *systemType.DataPrivacyRequestComment) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// federationExposedModuleTable represents federationExposedModules store table
	//
	// This value is auto-generated
	federationExposedModuleTable = goqu.T("federation_module_exposed")

	// federationExposedModuleSelectQuery assembles select query for fetching federationExposedModules
	//
	// This function is auto-generated
	federationExposedModuleSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"handle",
			"name",
			"rel_node",
			"rel_compose_module",
			"rel_compose_namespace",
			"fields",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(federationExposedModuleTable)
	}

	// federationExposedModuleInsertQuery assembles query inserting federationExposedModules
	//
	// This function is auto-generated
	federationExposedModuleInsertQuery = func(d goqu.DialectWrapper, res *federationType.ExposedModule) *goqu.InsertDataset {
		return d.Insert(federationExposedModuleTable).
			Rows(goqu.Record{
				"id":                    res.ID,
				"handle":                res.Handle,
				"name":                  res.Name,
				"rel_node":              res.NodeID,
				"rel_compose_module":    res.ComposeModuleID,
				"rel_compose_namespace": res.ComposeNamespaceID,
				"fields":                res.Fields,
				"created_at":            res.CreatedAt,
				"updated_at":            res.UpdatedAt,
				"deleted_at":            res.DeletedAt,
				"created_by":            res.CreatedBy,
				"updated_by":            res.UpdatedBy,
				"deleted_by":            res.DeletedBy,
			})
	}

	// federationExposedModuleUpsertQuery assembles (insert+on-conflict) query for replacing federationExposedModules
	//
	// This function is auto-generated
	federationExposedModuleUpsertQuery = func(d goqu.DialectWrapper, res *federationType.ExposedModule) *goqu.InsertDataset {
		var target = `,id`

		return federationExposedModuleInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"handle":                res.Handle,
						"name":                  res.Name,
						"rel_node":              res.NodeID,
						"rel_compose_module":    res.ComposeModuleID,
						"rel_compose_namespace": res.ComposeNamespaceID,
						"fields":                res.Fields,
						"created_at":            res.CreatedAt,
						"updated_at":            res.UpdatedAt,
						"deleted_at":            res.DeletedAt,
						"created_by":            res.CreatedBy,
						"updated_by":            res.UpdatedBy,
						"deleted_by":            res.DeletedBy,
					},
				),
			)
	}

	// federationExposedModuleUpdateQuery assembles query for updating federationExposedModules
	//
	// This function is auto-generated
	federationExposedModuleUpdateQuery = func(d goqu.DialectWrapper, res *federationType.ExposedModule) *goqu.UpdateDataset {
		return d.Update(federationExposedModuleTable).
			Set(goqu.Record{
				"handle":                res.Handle,
				"name":                  res.Name,
				"rel_node":              res.NodeID,
				"rel_compose_module":    res.ComposeModuleID,
				"rel_compose_namespace": res.ComposeNamespaceID,
				"fields":                res.Fields,
				"created_at":            res.CreatedAt,
				"updated_at":            res.UpdatedAt,
				"deleted_at":            res.DeletedAt,
				"created_by":            res.CreatedBy,
				"updated_by":            res.UpdatedBy,
				"deleted_by":            res.DeletedBy,
			}).
			Where(federationExposedModulePrimaryKeys(res))
	}

	// federationExposedModuleDeleteQuery assembles delete query for removing federationExposedModules
	//
	// This function is auto-generated
	federationExposedModuleDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(federationExposedModuleTable).Where(ee...)
	}

	// federationExposedModuleDeleteQuery assembles delete query for removing federationExposedModules
	//
	// This function is auto-generated
	federationExposedModuleTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(federationExposedModuleTable)
	}

	// federationExposedModulePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	federationExposedModulePrimaryKeys = func(res *federationType.ExposedModule) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// federationModuleMappingTable represents federationModuleMappings store table
	//
	// This value is auto-generated
	federationModuleMappingTable = goqu.T("federation_module_mapping")

	// federationModuleMappingSelectQuery assembles select query for fetching federationModuleMappings
	//
	// This function is auto-generated
	federationModuleMappingSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"node_id",
			"rel_federation_module",
			"rel_compose_module",
			"rel_compose_namespace",
			"field_mapping",
		).From(federationModuleMappingTable)
	}

	// federationModuleMappingInsertQuery assembles query inserting federationModuleMappings
	//
	// This function is auto-generated
	federationModuleMappingInsertQuery = func(d goqu.DialectWrapper, res *federationType.ModuleMapping) *goqu.InsertDataset {
		return d.Insert(federationModuleMappingTable).
			Rows(goqu.Record{
				"node_id":               res.NodeID,
				"rel_federation_module": res.FederationModuleID,
				"rel_compose_module":    res.ComposeModuleID,
				"rel_compose_namespace": res.ComposeNamespaceID,
				"field_mapping":         res.FieldMapping,
			})
	}

	// federationModuleMappingUpsertQuery assembles (insert+on-conflict) query for replacing federationModuleMappings
	//
	// This function is auto-generated
	federationModuleMappingUpsertQuery = func(d goqu.DialectWrapper, res *federationType.ModuleMapping) *goqu.InsertDataset {
		var target = ``

		return federationModuleMappingInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"node_id":               res.NodeID,
						"rel_federation_module": res.FederationModuleID,
						"rel_compose_module":    res.ComposeModuleID,
						"rel_compose_namespace": res.ComposeNamespaceID,
						"field_mapping":         res.FieldMapping,
					},
				),
			)
	}

	// federationModuleMappingUpdateQuery assembles query for updating federationModuleMappings
	//
	// This function is auto-generated
	federationModuleMappingUpdateQuery = func(d goqu.DialectWrapper, res *federationType.ModuleMapping) *goqu.UpdateDataset {
		return d.Update(federationModuleMappingTable).
			Set(goqu.Record{
				"node_id":               res.NodeID,
				"rel_federation_module": res.FederationModuleID,
				"rel_compose_module":    res.ComposeModuleID,
				"rel_compose_namespace": res.ComposeNamespaceID,
				"field_mapping":         res.FieldMapping,
			}).
			Where(federationModuleMappingPrimaryKeys(res))
	}

	// federationModuleMappingDeleteQuery assembles delete query for removing federationModuleMappings
	//
	// This function is auto-generated
	federationModuleMappingDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(federationModuleMappingTable).Where(ee...)
	}

	// federationModuleMappingDeleteQuery assembles delete query for removing federationModuleMappings
	//
	// This function is auto-generated
	federationModuleMappingTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(federationModuleMappingTable)
	}

	// federationModuleMappingPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	federationModuleMappingPrimaryKeys = func(res *federationType.ModuleMapping) goqu.Ex {
		return goqu.Ex{}
	}

	// federationNodeTable represents federationNodes store table
	//
	// This value is auto-generated
	federationNodeTable = goqu.T("federation_nodes")

	// federationNodeSelectQuery assembles select query for fetching federationNodes
	//
	// This function is auto-generated
	federationNodeSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"shared_node_id",
			"name",
			"base_url",
			"status",
			"contact",
			"pair_token",
			"auth_token",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(federationNodeTable)
	}

	// federationNodeInsertQuery assembles query inserting federationNodes
	//
	// This function is auto-generated
	federationNodeInsertQuery = func(d goqu.DialectWrapper, res *federationType.Node) *goqu.InsertDataset {
		return d.Insert(federationNodeTable).
			Rows(goqu.Record{
				"id":             res.ID,
				"shared_node_id": res.SharedNodeID,
				"name":           res.Name,
				"base_url":       res.BaseURL,
				"status":         res.Status,
				"contact":        res.Contact,
				"pair_token":     res.PairToken,
				"auth_token":     res.AuthToken,
				"created_at":     res.CreatedAt,
				"updated_at":     res.UpdatedAt,
				"deleted_at":     res.DeletedAt,
				"created_by":     res.CreatedBy,
				"updated_by":     res.UpdatedBy,
				"deleted_by":     res.DeletedBy,
			})
	}

	// federationNodeUpsertQuery assembles (insert+on-conflict) query for replacing federationNodes
	//
	// This function is auto-generated
	federationNodeUpsertQuery = func(d goqu.DialectWrapper, res *federationType.Node) *goqu.InsertDataset {
		var target = `,id`

		return federationNodeInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"shared_node_id": res.SharedNodeID,
						"name":           res.Name,
						"base_url":       res.BaseURL,
						"status":         res.Status,
						"contact":        res.Contact,
						"pair_token":     res.PairToken,
						"auth_token":     res.AuthToken,
						"created_at":     res.CreatedAt,
						"updated_at":     res.UpdatedAt,
						"deleted_at":     res.DeletedAt,
						"created_by":     res.CreatedBy,
						"updated_by":     res.UpdatedBy,
						"deleted_by":     res.DeletedBy,
					},
				),
			)
	}

	// federationNodeUpdateQuery assembles query for updating federationNodes
	//
	// This function is auto-generated
	federationNodeUpdateQuery = func(d goqu.DialectWrapper, res *federationType.Node) *goqu.UpdateDataset {
		return d.Update(federationNodeTable).
			Set(goqu.Record{
				"shared_node_id": res.SharedNodeID,
				"name":           res.Name,
				"base_url":       res.BaseURL,
				"status":         res.Status,
				"contact":        res.Contact,
				"pair_token":     res.PairToken,
				"auth_token":     res.AuthToken,
				"created_at":     res.CreatedAt,
				"updated_at":     res.UpdatedAt,
				"deleted_at":     res.DeletedAt,
				"created_by":     res.CreatedBy,
				"updated_by":     res.UpdatedBy,
				"deleted_by":     res.DeletedBy,
			}).
			Where(federationNodePrimaryKeys(res))
	}

	// federationNodeDeleteQuery assembles delete query for removing federationNodes
	//
	// This function is auto-generated
	federationNodeDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(federationNodeTable).Where(ee...)
	}

	// federationNodeDeleteQuery assembles delete query for removing federationNodes
	//
	// This function is auto-generated
	federationNodeTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(federationNodeTable)
	}

	// federationNodePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	federationNodePrimaryKeys = func(res *federationType.Node) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// federationNodeSyncTable represents federationNodeSyncs store table
	//
	// This value is auto-generated
	federationNodeSyncTable = goqu.T("federation_nodes_sync")

	// federationNodeSyncSelectQuery assembles select query for fetching federationNodeSyncs
	//
	// This function is auto-generated
	federationNodeSyncSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"rel_node",
			"rel_compose_module",
			"sync_type",
			"sync_status",
			"time_of_action",
		).From(federationNodeSyncTable)
	}

	// federationNodeSyncInsertQuery assembles query inserting federationNodeSyncs
	//
	// This function is auto-generated
	federationNodeSyncInsertQuery = func(d goqu.DialectWrapper, res *federationType.NodeSync) *goqu.InsertDataset {
		return d.Insert(federationNodeSyncTable).
			Rows(goqu.Record{
				"rel_node":           res.NodeID,
				"rel_compose_module": res.ModuleID,
				"sync_type":          res.SyncType,
				"sync_status":        res.SyncStatus,
				"time_of_action":     res.TimeOfAction,
			})
	}

	// federationNodeSyncUpsertQuery assembles (insert+on-conflict) query for replacing federationNodeSyncs
	//
	// This function is auto-generated
	federationNodeSyncUpsertQuery = func(d goqu.DialectWrapper, res *federationType.NodeSync) *goqu.InsertDataset {
		var target = ``

		return federationNodeSyncInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"rel_node":           res.NodeID,
						"rel_compose_module": res.ModuleID,
						"sync_type":          res.SyncType,
						"sync_status":        res.SyncStatus,
						"time_of_action":     res.TimeOfAction,
					},
				),
			)
	}

	// federationNodeSyncUpdateQuery assembles query for updating federationNodeSyncs
	//
	// This function is auto-generated
	federationNodeSyncUpdateQuery = func(d goqu.DialectWrapper, res *federationType.NodeSync) *goqu.UpdateDataset {
		return d.Update(federationNodeSyncTable).
			Set(goqu.Record{
				"rel_node":           res.NodeID,
				"rel_compose_module": res.ModuleID,
				"sync_type":          res.SyncType,
				"sync_status":        res.SyncStatus,
				"time_of_action":     res.TimeOfAction,
			}).
			Where(federationNodeSyncPrimaryKeys(res))
	}

	// federationNodeSyncDeleteQuery assembles delete query for removing federationNodeSyncs
	//
	// This function is auto-generated
	federationNodeSyncDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(federationNodeSyncTable).Where(ee...)
	}

	// federationNodeSyncDeleteQuery assembles delete query for removing federationNodeSyncs
	//
	// This function is auto-generated
	federationNodeSyncTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(federationNodeSyncTable)
	}

	// federationNodeSyncPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	federationNodeSyncPrimaryKeys = func(res *federationType.NodeSync) goqu.Ex {
		return goqu.Ex{}
	}

	// federationSharedModuleTable represents federationSharedModules store table
	//
	// This value is auto-generated
	federationSharedModuleTable = goqu.T("federation_module_shared")

	// federationSharedModuleSelectQuery assembles select query for fetching federationSharedModules
	//
	// This function is auto-generated
	federationSharedModuleSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"handle",
			"rel_node",
			"name",
			"xref_module",
			"fields",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(federationSharedModuleTable)
	}

	// federationSharedModuleInsertQuery assembles query inserting federationSharedModules
	//
	// This function is auto-generated
	federationSharedModuleInsertQuery = func(d goqu.DialectWrapper, res *federationType.SharedModule) *goqu.InsertDataset {
		return d.Insert(federationSharedModuleTable).
			Rows(goqu.Record{
				"id":          res.ID,
				"handle":      res.Handle,
				"rel_node":    res.NodeID,
				"name":        res.Name,
				"xref_module": res.ExternalFederationModuleID,
				"fields":      res.Fields,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
				"deleted_at":  res.DeletedAt,
				"created_by":  res.CreatedBy,
				"updated_by":  res.UpdatedBy,
				"deleted_by":  res.DeletedBy,
			})
	}

	// federationSharedModuleUpsertQuery assembles (insert+on-conflict) query for replacing federationSharedModules
	//
	// This function is auto-generated
	federationSharedModuleUpsertQuery = func(d goqu.DialectWrapper, res *federationType.SharedModule) *goqu.InsertDataset {
		var target = `,id`

		return federationSharedModuleInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"handle":      res.Handle,
						"rel_node":    res.NodeID,
						"name":        res.Name,
						"xref_module": res.ExternalFederationModuleID,
						"fields":      res.Fields,
						"created_at":  res.CreatedAt,
						"updated_at":  res.UpdatedAt,
						"deleted_at":  res.DeletedAt,
						"created_by":  res.CreatedBy,
						"updated_by":  res.UpdatedBy,
						"deleted_by":  res.DeletedBy,
					},
				),
			)
	}

	// federationSharedModuleUpdateQuery assembles query for updating federationSharedModules
	//
	// This function is auto-generated
	federationSharedModuleUpdateQuery = func(d goqu.DialectWrapper, res *federationType.SharedModule) *goqu.UpdateDataset {
		return d.Update(federationSharedModuleTable).
			Set(goqu.Record{
				"handle":      res.Handle,
				"rel_node":    res.NodeID,
				"name":        res.Name,
				"xref_module": res.ExternalFederationModuleID,
				"fields":      res.Fields,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
				"deleted_at":  res.DeletedAt,
				"created_by":  res.CreatedBy,
				"updated_by":  res.UpdatedBy,
				"deleted_by":  res.DeletedBy,
			}).
			Where(federationSharedModulePrimaryKeys(res))
	}

	// federationSharedModuleDeleteQuery assembles delete query for removing federationSharedModules
	//
	// This function is auto-generated
	federationSharedModuleDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(federationSharedModuleTable).Where(ee...)
	}

	// federationSharedModuleDeleteQuery assembles delete query for removing federationSharedModules
	//
	// This function is auto-generated
	federationSharedModuleTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(federationSharedModuleTable)
	}

	// federationSharedModulePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	federationSharedModulePrimaryKeys = func(res *federationType.SharedModule) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// flagTable represents flags store table
	//
	// This value is auto-generated
	flagTable = goqu.T("flags")

	// flagSelectQuery assembles select query for fetching flags
	//
	// This function is auto-generated
	flagSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"kind",
			"rel_resource",
			"owned_by",
			"name",
			"active",
		).From(flagTable)
	}

	// flagInsertQuery assembles query inserting flags
	//
	// This function is auto-generated
	flagInsertQuery = func(d goqu.DialectWrapper, res *flagType.Flag) *goqu.InsertDataset {
		return d.Insert(flagTable).
			Rows(goqu.Record{
				"kind":         res.Kind,
				"rel_resource": res.ResourceID,
				"owned_by":     res.OwnedBy,
				"name":         res.Name,
				"active":       res.Active,
			})
	}

	// flagUpsertQuery assembles (insert+on-conflict) query for replacing flags
	//
	// This function is auto-generated
	flagUpsertQuery = func(d goqu.DialectWrapper, res *flagType.Flag) *goqu.InsertDataset {
		var target = `,kind,rel_resource,owned_by,LOWER(name)`

		return flagInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"active": res.Active,
					},
				),
			)
	}

	// flagUpdateQuery assembles query for updating flags
	//
	// This function is auto-generated
	flagUpdateQuery = func(d goqu.DialectWrapper, res *flagType.Flag) *goqu.UpdateDataset {
		return d.Update(flagTable).
			Set(goqu.Record{
				"active": res.Active,
			}).
			Where(flagPrimaryKeys(res))
	}

	// flagDeleteQuery assembles delete query for removing flags
	//
	// This function is auto-generated
	flagDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(flagTable).Where(ee...)
	}

	// flagDeleteQuery assembles delete query for removing flags
	//
	// This function is auto-generated
	flagTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(flagTable)
	}

	// flagPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	flagPrimaryKeys = func(res *flagType.Flag) goqu.Ex {
		return goqu.Ex{
			"kind":         res.Kind,
			"rel_resource": res.ResourceID,
			"owned_by":     res.OwnedBy,
			"name":         res.Name,
		}
	}

	// labelTable represents labels store table
	//
	// This value is auto-generated
	labelTable = goqu.T("labels")

	// labelSelectQuery assembles select query for fetching labels
	//
	// This function is auto-generated
	labelSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"kind",
			"rel_resource",
			"name",
			"value",
		).From(labelTable)
	}

	// labelInsertQuery assembles query inserting labels
	//
	// This function is auto-generated
	labelInsertQuery = func(d goqu.DialectWrapper, res *labelsType.Label) *goqu.InsertDataset {
		return d.Insert(labelTable).
			Rows(goqu.Record{
				"kind":         res.Kind,
				"rel_resource": res.ResourceID,
				"name":         res.Name,
				"value":        res.Value,
			})
	}

	// labelUpsertQuery assembles (insert+on-conflict) query for replacing labels
	//
	// This function is auto-generated
	labelUpsertQuery = func(d goqu.DialectWrapper, res *labelsType.Label) *goqu.InsertDataset {
		var target = `,kind,rel_resource,LOWER(name)`

		return labelInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"value": res.Value,
					},
				),
			)
	}

	// labelUpdateQuery assembles query for updating labels
	//
	// This function is auto-generated
	labelUpdateQuery = func(d goqu.DialectWrapper, res *labelsType.Label) *goqu.UpdateDataset {
		return d.Update(labelTable).
			Set(goqu.Record{
				"value": res.Value,
			}).
			Where(labelPrimaryKeys(res))
	}

	// labelDeleteQuery assembles delete query for removing labels
	//
	// This function is auto-generated
	labelDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(labelTable).Where(ee...)
	}

	// labelDeleteQuery assembles delete query for removing labels
	//
	// This function is auto-generated
	labelTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(labelTable)
	}

	// labelPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	labelPrimaryKeys = func(res *labelsType.Label) goqu.Ex {
		return goqu.Ex{
			"kind":         res.Kind,
			"rel_resource": res.ResourceID,
			"name":         res.Name,
		}
	}

	// notificationTable represents notifications store table
	//
	// This value is auto-generated
	notificationTable = goqu.T("notifications")

	// notificationSelectQuery assembles select query for fetching notifications
	//
	// This function is auto-generated
	notificationSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"kind",
			"config",
			"recipient",
			"created_by",
			"read_at",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(notificationTable)
	}

	// notificationInsertQuery assembles query inserting notifications
	//
	// This function is auto-generated
	notificationInsertQuery = func(d goqu.DialectWrapper, res *systemType.Notification) *goqu.InsertDataset {
		return d.Insert(notificationTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"kind":       res.Kind,
				"config":     res.Config,
				"recipient":  res.Recipient,
				"created_by": res.CreatedBy,
				"read_at":    res.ReadAt,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
			})
	}

	// notificationUpsertQuery assembles (insert+on-conflict) query for replacing notifications
	//
	// This function is auto-generated
	notificationUpsertQuery = func(d goqu.DialectWrapper, res *systemType.Notification) *goqu.InsertDataset {
		var target = `,id`

		return notificationInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"kind":       res.Kind,
						"config":     res.Config,
						"recipient":  res.Recipient,
						"created_by": res.CreatedBy,
						"read_at":    res.ReadAt,
						"created_at": res.CreatedAt,
						"updated_at": res.UpdatedAt,
						"deleted_at": res.DeletedAt,
					},
				),
			)
	}

	// notificationUpdateQuery assembles query for updating notifications
	//
	// This function is auto-generated
	notificationUpdateQuery = func(d goqu.DialectWrapper, res *systemType.Notification) *goqu.UpdateDataset {
		return d.Update(notificationTable).
			Set(goqu.Record{
				"kind":       res.Kind,
				"config":     res.Config,
				"recipient":  res.Recipient,
				"created_by": res.CreatedBy,
				"read_at":    res.ReadAt,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
			}).
			Where(notificationPrimaryKeys(res))
	}

	// notificationDeleteQuery assembles delete query for removing notifications
	//
	// This function is auto-generated
	notificationDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(notificationTable).Where(ee...)
	}

	// notificationDeleteQuery assembles delete query for removing notifications
	//
	// This function is auto-generated
	notificationTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(notificationTable)
	}

	// notificationPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	notificationPrimaryKeys = func(res *systemType.Notification) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// queueTable represents queues store table
	//
	// This value is auto-generated
	queueTable = goqu.T("queue_settings")

	// queueSelectQuery assembles select query for fetching queues
	//
	// This function is auto-generated
	queueSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"consumer",
			"queue",
			"meta",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(queueTable)
	}

	// queueInsertQuery assembles query inserting queues
	//
	// This function is auto-generated
	queueInsertQuery = func(d goqu.DialectWrapper, res *systemType.Queue) *goqu.InsertDataset {
		return d.Insert(queueTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"consumer":   res.Consumer,
				"queue":      res.Queue,
				"meta":       res.Meta,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			})
	}

	// queueUpsertQuery assembles (insert+on-conflict) query for replacing queues
	//
	// This function is auto-generated
	queueUpsertQuery = func(d goqu.DialectWrapper, res *systemType.Queue) *goqu.InsertDataset {
		var target = `,id`

		return queueInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"consumer":   res.Consumer,
						"queue":      res.Queue,
						"meta":       res.Meta,
						"created_at": res.CreatedAt,
						"updated_at": res.UpdatedAt,
						"deleted_at": res.DeletedAt,
						"created_by": res.CreatedBy,
						"updated_by": res.UpdatedBy,
						"deleted_by": res.DeletedBy,
					},
				),
			)
	}

	// queueUpdateQuery assembles query for updating queues
	//
	// This function is auto-generated
	queueUpdateQuery = func(d goqu.DialectWrapper, res *systemType.Queue) *goqu.UpdateDataset {
		return d.Update(queueTable).
			Set(goqu.Record{
				"consumer":   res.Consumer,
				"queue":      res.Queue,
				"meta":       res.Meta,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			}).
			Where(queuePrimaryKeys(res))
	}

	// queueDeleteQuery assembles delete query for removing queues
	//
	// This function is auto-generated
	queueDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(queueTable).Where(ee...)
	}

	// queueDeleteQuery assembles delete query for removing queues
	//
	// This function is auto-generated
	queueTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(queueTable)
	}

	// queuePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	queuePrimaryKeys = func(res *systemType.Queue) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// queueMessageTable represents queueMessages store table
	//
	// This value is auto-generated
	queueMessageTable = goqu.T("queue_messages")

	// queueMessageSelectQuery assembles select query for fetching queueMessages
	//
	// This function is auto-generated
	queueMessageSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"queue",
			"payload",
			"created",
			"processed",
		).From(queueMessageTable)
	}

	// queueMessageInsertQuery assembles query inserting queueMessages
	//
	// This function is auto-generated
	queueMessageInsertQuery = func(d goqu.DialectWrapper, res *systemType.QueueMessage) *goqu.InsertDataset {
		return d.Insert(queueMessageTable).
			Rows(goqu.Record{
				"id":        res.ID,
				"queue":     res.Queue,
				"payload":   res.Payload,
				"created":   res.Created,
				"processed": res.Processed,
			})
	}

	// queueMessageUpsertQuery assembles (insert+on-conflict) query for replacing queueMessages
	//
	// This function is auto-generated
	queueMessageUpsertQuery = func(d goqu.DialectWrapper, res *systemType.QueueMessage) *goqu.InsertDataset {
		var target = `,id`

		return queueMessageInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"queue":     res.Queue,
						"payload":   res.Payload,
						"created":   res.Created,
						"processed": res.Processed,
					},
				),
			)
	}

	// queueMessageUpdateQuery assembles query for updating queueMessages
	//
	// This function is auto-generated
	queueMessageUpdateQuery = func(d goqu.DialectWrapper, res *systemType.QueueMessage) *goqu.UpdateDataset {
		return d.Update(queueMessageTable).
			Set(goqu.Record{
				"queue":     res.Queue,
				"payload":   res.Payload,
				"created":   res.Created,
				"processed": res.Processed,
			}).
			Where(queueMessagePrimaryKeys(res))
	}

	// queueMessageDeleteQuery assembles delete query for removing queueMessages
	//
	// This function is auto-generated
	queueMessageDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(queueMessageTable).Where(ee...)
	}

	// queueMessageDeleteQuery assembles delete query for removing queueMessages
	//
	// This function is auto-generated
	queueMessageTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(queueMessageTable)
	}

	// queueMessagePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	queueMessagePrimaryKeys = func(res *systemType.QueueMessage) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// rbacRuleTable represents rbacRules store table
	//
	// This value is auto-generated
	rbacRuleTable = goqu.T("rbac_rules")

	// rbacRuleSelectQuery assembles select query for fetching rbacRules
	//
	// This function is auto-generated
	rbacRuleSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"rel_role",
			"resource",
			"operation",
			"access",
		).From(rbacRuleTable)
	}

	// rbacRuleInsertQuery assembles query inserting rbacRules
	//
	// This function is auto-generated
	rbacRuleInsertQuery = func(d goqu.DialectWrapper, res *rbacType.Rule) *goqu.InsertDataset {
		return d.Insert(rbacRuleTable).
			Rows(goqu.Record{
				"rel_role":  res.RoleID,
				"resource":  res.Resource,
				"operation": res.Operation,
				"access":    res.Access,
			})
	}

	// rbacRuleUpsertQuery assembles (insert+on-conflict) query for replacing rbacRules
	//
	// This function is auto-generated
	rbacRuleUpsertQuery = func(d goqu.DialectWrapper, res *rbacType.Rule) *goqu.InsertDataset {
		var target = `,rel_role,resource,operation`

		return rbacRuleInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"access": res.Access,
					},
				),
			)
	}

	// rbacRuleUpdateQuery assembles query for updating rbacRules
	//
	// This function is auto-generated
	rbacRuleUpdateQuery = func(d goqu.DialectWrapper, res *rbacType.Rule) *goqu.UpdateDataset {
		return d.Update(rbacRuleTable).
			Set(goqu.Record{
				"access": res.Access,
			}).
			Where(rbacRulePrimaryKeys(res))
	}

	// rbacRuleDeleteQuery assembles delete query for removing rbacRules
	//
	// This function is auto-generated
	rbacRuleDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(rbacRuleTable).Where(ee...)
	}

	// rbacRuleDeleteQuery assembles delete query for removing rbacRules
	//
	// This function is auto-generated
	rbacRuleTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(rbacRuleTable)
	}

	// rbacRulePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	rbacRulePrimaryKeys = func(res *rbacType.Rule) goqu.Ex {
		return goqu.Ex{
			"rel_role":  res.RoleID,
			"resource":  res.Resource,
			"operation": res.Operation,
		}
	}

	// reminderTable represents reminders store table
	//
	// This value is auto-generated
	reminderTable = goqu.T("reminders")

	// reminderSelectQuery assembles select query for fetching reminders
	//
	// This function is auto-generated
	reminderSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"resource",
			"payload",
			"snooze_count",
			"assigned_to",
			"assigned_by",
			"assigned_at",
			"dismissed_by",
			"dismissed_at",
			"remind_at",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(reminderTable)
	}

	// reminderInsertQuery assembles query inserting reminders
	//
	// This function is auto-generated
	reminderInsertQuery = func(d goqu.DialectWrapper, res *systemType.Reminder) *goqu.InsertDataset {
		return d.Insert(reminderTable).
			Rows(goqu.Record{
				"id":           res.ID,
				"resource":     res.Resource,
				"payload":      res.Payload,
				"snooze_count": res.SnoozeCount,
				"assigned_to":  res.AssignedTo,
				"assigned_by":  res.AssignedBy,
				"assigned_at":  res.AssignedAt,
				"dismissed_by": res.DismissedBy,
				"dismissed_at": res.DismissedAt,
				"remind_at":    res.RemindAt,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"deleted_at":   res.DeletedAt,
			})
	}

	// reminderUpsertQuery assembles (insert+on-conflict) query for replacing reminders
	//
	// This function is auto-generated
	reminderUpsertQuery = func(d goqu.DialectWrapper, res *systemType.Reminder) *goqu.InsertDataset {
		var target = `,id`

		return reminderInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"resource":     res.Resource,
						"payload":      res.Payload,
						"snooze_count": res.SnoozeCount,
						"assigned_to":  res.AssignedTo,
						"assigned_by":  res.AssignedBy,
						"assigned_at":  res.AssignedAt,
						"dismissed_by": res.DismissedBy,
						"dismissed_at": res.DismissedAt,
						"remind_at":    res.RemindAt,
						"created_at":   res.CreatedAt,
						"updated_at":   res.UpdatedAt,
						"deleted_at":   res.DeletedAt,
					},
				),
			)
	}

	// reminderUpdateQuery assembles query for updating reminders
	//
	// This function is auto-generated
	reminderUpdateQuery = func(d goqu.DialectWrapper, res *systemType.Reminder) *goqu.UpdateDataset {
		return d.Update(reminderTable).
			Set(goqu.Record{
				"resource":     res.Resource,
				"payload":      res.Payload,
				"snooze_count": res.SnoozeCount,
				"assigned_to":  res.AssignedTo,
				"assigned_by":  res.AssignedBy,
				"assigned_at":  res.AssignedAt,
				"dismissed_by": res.DismissedBy,
				"dismissed_at": res.DismissedAt,
				"remind_at":    res.RemindAt,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"deleted_at":   res.DeletedAt,
			}).
			Where(reminderPrimaryKeys(res))
	}

	// reminderDeleteQuery assembles delete query for removing reminders
	//
	// This function is auto-generated
	reminderDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(reminderTable).Where(ee...)
	}

	// reminderDeleteQuery assembles delete query for removing reminders
	//
	// This function is auto-generated
	reminderTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(reminderTable)
	}

	// reminderPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	reminderPrimaryKeys = func(res *systemType.Reminder) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// reportTable represents reports store table
	//
	// This value is auto-generated
	reportTable = goqu.T("reports")

	// reportSelectQuery assembles select query for fetching reports
	//
	// This function is auto-generated
	reportSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"handle",
			"meta",
			"scenarios",
			"sources",
			"blocks",
			"owned_by",
			"created_at",
			"updated_at",
			"deleted_at",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(reportTable)
	}

	// reportInsertQuery assembles query inserting reports
	//
	// This function is auto-generated
	reportInsertQuery = func(d goqu.DialectWrapper, res *systemType.Report) *goqu.InsertDataset {
		return d.Insert(reportTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"handle":     res.Handle,
				"meta":       res.Meta,
				"scenarios":  res.Scenarios,
				"sources":    res.Sources,
				"blocks":     res.Blocks,
				"owned_by":   res.OwnedBy,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			})
	}

	// reportUpsertQuery assembles (insert+on-conflict) query for replacing reports
	//
	// This function is auto-generated
	reportUpsertQuery = func(d goqu.DialectWrapper, res *systemType.Report) *goqu.InsertDataset {
		var target = `,id`

		return reportInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"handle":     res.Handle,
						"meta":       res.Meta,
						"scenarios":  res.Scenarios,
						"sources":    res.Sources,
						"blocks":     res.Blocks,
						"owned_by":   res.OwnedBy,
						"created_at": res.CreatedAt,
						"updated_at": res.UpdatedAt,
						"deleted_at": res.DeletedAt,
						"created_by": res.CreatedBy,
						"updated_by": res.UpdatedBy,
						"deleted_by": res.DeletedBy,
					},
				),
			)
	}

	// reportUpdateQuery assembles query for updating reports
	//
	// This function is auto-generated
	reportUpdateQuery = func(d goqu.DialectWrapper, res *systemType.Report) *goqu.UpdateDataset {
		return d.Update(reportTable).
			Set(goqu.Record{
				"handle":     res.Handle,
				"meta":       res.Meta,
				"scenarios":  res.Scenarios,
				"sources":    res.Sources,
				"blocks":     res.Blocks,
				"owned_by":   res.OwnedBy,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			}).
			Where(reportPrimaryKeys(res))
	}

	// reportDeleteQuery assembles delete query for removing reports
	//
	// This function is auto-generated
	reportDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(reportTable).Where(ee...)
	}

	// reportDeleteQuery assembles delete query for removing reports
	//
	// This function is auto-generated
	reportTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(reportTable)
	}

	// reportPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	reportPrimaryKeys = func(res *systemType.Report) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// resourceActivityTable represents resourceActivitys store table
	//
	// This value is auto-generated
	resourceActivityTable = goqu.T("resource_activity_log")

	// resourceActivitySelectQuery assembles select query for fetching resourceActivitys
	//
	// This function is auto-generated
	resourceActivitySelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"ts",
			"resource_type",
			"resource_action",
			"rel_resource",
			"meta",
		).From(resourceActivityTable)
	}

	// resourceActivityInsertQuery assembles query inserting resourceActivitys
	//
	// This function is auto-generated
	resourceActivityInsertQuery = func(d goqu.DialectWrapper, res *discoveryType.ResourceActivity) *goqu.InsertDataset {
		return d.Insert(resourceActivityTable).
			Rows(goqu.Record{
				"id":              res.ID,
				"ts":              res.Timestamp,
				"resource_type":   res.ResourceType,
				"resource_action": res.ResourceAction,
				"rel_resource":    res.ResourceID,
				"meta":            res.Meta,
			})
	}

	// resourceActivityUpsertQuery assembles (insert+on-conflict) query for replacing resourceActivitys
	//
	// This function is auto-generated
	resourceActivityUpsertQuery = func(d goqu.DialectWrapper, res *discoveryType.ResourceActivity) *goqu.InsertDataset {
		var target = `,id`

		return resourceActivityInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"ts":              res.Timestamp,
						"resource_type":   res.ResourceType,
						"resource_action": res.ResourceAction,
						"rel_resource":    res.ResourceID,
						"meta":            res.Meta,
					},
				),
			)
	}

	// resourceActivityUpdateQuery assembles query for updating resourceActivitys
	//
	// This function is auto-generated
	resourceActivityUpdateQuery = func(d goqu.DialectWrapper, res *discoveryType.ResourceActivity) *goqu.UpdateDataset {
		return d.Update(resourceActivityTable).
			Set(goqu.Record{
				"ts":              res.Timestamp,
				"resource_type":   res.ResourceType,
				"resource_action": res.ResourceAction,
				"rel_resource":    res.ResourceID,
				"meta":            res.Meta,
			}).
			Where(resourceActivityPrimaryKeys(res))
	}

	// resourceActivityDeleteQuery assembles delete query for removing resourceActivitys
	//
	// This function is auto-generated
	resourceActivityDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(resourceActivityTable).Where(ee...)
	}

	// resourceActivityDeleteQuery assembles delete query for removing resourceActivitys
	//
	// This function is auto-generated
	resourceActivityTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(resourceActivityTable)
	}

	// resourceActivityPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	resourceActivityPrimaryKeys = func(res *discoveryType.ResourceActivity) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// resourceTranslationTable represents resourceTranslations store table
	//
	// This value is auto-generated
	resourceTranslationTable = goqu.T("resource_translations")

	// resourceTranslationSelectQuery assembles select query for fetching resourceTranslations
	//
	// This function is auto-generated
	resourceTranslationSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"lang",
			"resource",
			"k",
			"message",
			"created_at",
			"updated_at",
			"deleted_at",
			"owned_by",
			"created_by",
			"updated_by",
			"deleted_by",
		).From(resourceTranslationTable)
	}

	// resourceTranslationInsertQuery assembles query inserting resourceTranslations
	//
	// This function is auto-generated
	resourceTranslationInsertQuery = func(d goqu.DialectWrapper, res *systemType.ResourceTranslation) *goqu.InsertDataset {
		return d.Insert(resourceTranslationTable).
			Rows(goqu.Record{
				"id":         res.ID,
				"lang":       res.Lang,
				"resource":   res.Resource,
				"k":          res.K,
				"message":    res.Message,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"owned_by":   res.OwnedBy,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			})
	}

	// resourceTranslationUpsertQuery assembles (insert+on-conflict) query for replacing resourceTranslations
	//
	// This function is auto-generated
	resourceTranslationUpsertQuery = func(d goqu.DialectWrapper, res *systemType.ResourceTranslation) *goqu.InsertDataset {
		var target = `,id`

		return resourceTranslationInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"lang":       res.Lang,
						"resource":   res.Resource,
						"k":          res.K,
						"message":    res.Message,
						"created_at": res.CreatedAt,
						"updated_at": res.UpdatedAt,
						"deleted_at": res.DeletedAt,
						"owned_by":   res.OwnedBy,
						"created_by": res.CreatedBy,
						"updated_by": res.UpdatedBy,
						"deleted_by": res.DeletedBy,
					},
				),
			)
	}

	// resourceTranslationUpdateQuery assembles query for updating resourceTranslations
	//
	// This function is auto-generated
	resourceTranslationUpdateQuery = func(d goqu.DialectWrapper, res *systemType.ResourceTranslation) *goqu.UpdateDataset {
		return d.Update(resourceTranslationTable).
			Set(goqu.Record{
				"lang":       res.Lang,
				"resource":   res.Resource,
				"k":          res.K,
				"message":    res.Message,
				"created_at": res.CreatedAt,
				"updated_at": res.UpdatedAt,
				"deleted_at": res.DeletedAt,
				"owned_by":   res.OwnedBy,
				"created_by": res.CreatedBy,
				"updated_by": res.UpdatedBy,
				"deleted_by": res.DeletedBy,
			}).
			Where(resourceTranslationPrimaryKeys(res))
	}

	// resourceTranslationDeleteQuery assembles delete query for removing resourceTranslations
	//
	// This function is auto-generated
	resourceTranslationDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(resourceTranslationTable).Where(ee...)
	}

	// resourceTranslationDeleteQuery assembles delete query for removing resourceTranslations
	//
	// This function is auto-generated
	resourceTranslationTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(resourceTranslationTable)
	}

	// resourceTranslationPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	resourceTranslationPrimaryKeys = func(res *systemType.ResourceTranslation) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// roleTable represents roles store table
	//
	// This value is auto-generated
	roleTable = goqu.T("roles")

	// roleSelectQuery assembles select query for fetching roles
	//
	// This function is auto-generated
	roleSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"name",
			"handle",
			"meta",
			"archived_at",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(roleTable)
	}

	// roleInsertQuery assembles query inserting roles
	//
	// This function is auto-generated
	roleInsertQuery = func(d goqu.DialectWrapper, res *systemType.Role) *goqu.InsertDataset {
		return d.Insert(roleTable).
			Rows(goqu.Record{
				"id":          res.ID,
				"name":        res.Name,
				"handle":      res.Handle,
				"meta":        res.Meta,
				"archived_at": res.ArchivedAt,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
				"deleted_at":  res.DeletedAt,
			})
	}

	// roleUpsertQuery assembles (insert+on-conflict) query for replacing roles
	//
	// This function is auto-generated
	roleUpsertQuery = func(d goqu.DialectWrapper, res *systemType.Role) *goqu.InsertDataset {
		var target = `,id`

		return roleInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"name":        res.Name,
						"handle":      res.Handle,
						"meta":        res.Meta,
						"archived_at": res.ArchivedAt,
						"created_at":  res.CreatedAt,
						"updated_at":  res.UpdatedAt,
						"deleted_at":  res.DeletedAt,
					},
				),
			)
	}

	// roleUpdateQuery assembles query for updating roles
	//
	// This function is auto-generated
	roleUpdateQuery = func(d goqu.DialectWrapper, res *systemType.Role) *goqu.UpdateDataset {
		return d.Update(roleTable).
			Set(goqu.Record{
				"name":        res.Name,
				"handle":      res.Handle,
				"meta":        res.Meta,
				"archived_at": res.ArchivedAt,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
				"deleted_at":  res.DeletedAt,
			}).
			Where(rolePrimaryKeys(res))
	}

	// roleDeleteQuery assembles delete query for removing roles
	//
	// This function is auto-generated
	roleDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(roleTable).Where(ee...)
	}

	// roleDeleteQuery assembles delete query for removing roles
	//
	// This function is auto-generated
	roleTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(roleTable)
	}

	// rolePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	rolePrimaryKeys = func(res *systemType.Role) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// roleMemberTable represents roleMembers store table
	//
	// This value is auto-generated
	roleMemberTable = goqu.T("role_members")

	// roleMemberSelectQuery assembles select query for fetching roleMembers
	//
	// This function is auto-generated
	roleMemberSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"rel_resource",
			"rel_role",
		).From(roleMemberTable)
	}

	// roleMemberInsertQuery assembles query inserting roleMembers
	//
	// This function is auto-generated
	roleMemberInsertQuery = func(d goqu.DialectWrapper, res *systemType.RoleMember) *goqu.InsertDataset {
		return d.Insert(roleMemberTable).
			Rows(goqu.Record{
				"rel_resource": res.Resource,
				"rel_role":     res.RoleID,
			})
	}

	// roleMemberUpsertQuery assembles (insert+on-conflict) query for replacing roleMembers
	//
	// This function is auto-generated
	roleMemberUpsertQuery = func(d goqu.DialectWrapper, res *systemType.RoleMember) *goqu.InsertDataset {
		var target = `,rel_resource,rel_role`

		return roleMemberInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{},
				),
			)
	}

	// roleMemberUpdateQuery assembles query for updating roleMembers
	//
	// This function is auto-generated
	roleMemberUpdateQuery = func(d goqu.DialectWrapper, res *systemType.RoleMember) *goqu.UpdateDataset {
		return d.Update(roleMemberTable).
			Set(goqu.Record{}).
			Where(roleMemberPrimaryKeys(res))
	}

	// roleMemberDeleteQuery assembles delete query for removing roleMembers
	//
	// This function is auto-generated
	roleMemberDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(roleMemberTable).Where(ee...)
	}

	// roleMemberDeleteQuery assembles delete query for removing roleMembers
	//
	// This function is auto-generated
	roleMemberTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(roleMemberTable)
	}

	// roleMemberPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	roleMemberPrimaryKeys = func(res *systemType.RoleMember) goqu.Ex {
		return goqu.Ex{
			"rel_resource": res.Resource,
			"rel_role":     res.RoleID,
		}
	}

	// settingValueTable represents settingValues store table
	//
	// This value is auto-generated
	settingValueTable = goqu.T("settings")

	// settingValueSelectQuery assembles select query for fetching settingValues
	//
	// This function is auto-generated
	settingValueSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"rel_owner",
			"name",
			"value",
			"updated_by",
			"updated_at",
		).From(settingValueTable)
	}

	// settingValueInsertQuery assembles query inserting settingValues
	//
	// This function is auto-generated
	settingValueInsertQuery = func(d goqu.DialectWrapper, res *systemType.SettingValue) *goqu.InsertDataset {
		return d.Insert(settingValueTable).
			Rows(goqu.Record{
				"rel_owner":  res.OwnedBy,
				"name":       res.Name,
				"value":      res.Value,
				"updated_by": res.UpdatedBy,
				"updated_at": res.UpdatedAt,
			})
	}

	// settingValueUpsertQuery assembles (insert+on-conflict) query for replacing settingValues
	//
	// This function is auto-generated
	settingValueUpsertQuery = func(d goqu.DialectWrapper, res *systemType.SettingValue) *goqu.InsertDataset {
		var target = `,rel_owner,name`

		return settingValueInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"value":      res.Value,
						"updated_by": res.UpdatedBy,
						"updated_at": res.UpdatedAt,
					},
				),
			)
	}

	// settingValueUpdateQuery assembles query for updating settingValues
	//
	// This function is auto-generated
	settingValueUpdateQuery = func(d goqu.DialectWrapper, res *systemType.SettingValue) *goqu.UpdateDataset {
		return d.Update(settingValueTable).
			Set(goqu.Record{
				"value":      res.Value,
				"updated_by": res.UpdatedBy,
				"updated_at": res.UpdatedAt,
			}).
			Where(settingValuePrimaryKeys(res))
	}

	// settingValueDeleteQuery assembles delete query for removing settingValues
	//
	// This function is auto-generated
	settingValueDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(settingValueTable).Where(ee...)
	}

	// settingValueDeleteQuery assembles delete query for removing settingValues
	//
	// This function is auto-generated
	settingValueTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(settingValueTable)
	}

	// settingValuePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	settingValuePrimaryKeys = func(res *systemType.SettingValue) goqu.Ex {
		return goqu.Ex{
			"rel_owner": res.OwnedBy,
			"name":      res.Name,
		}
	}

	// templateTable represents templates store table
	//
	// This value is auto-generated
	templateTable = goqu.T("templates")

	// templateSelectQuery assembles select query for fetching templates
	//
	// This function is auto-generated
	templateSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"rel_owner",
			"handle",
			"language",
			"type",
			"partial",
			"meta",
			"template",
			"created_at",
			"updated_at",
			"deleted_at",
			"last_used_at",
		).From(templateTable)
	}

	// templateInsertQuery assembles query inserting templates
	//
	// This function is auto-generated
	templateInsertQuery = func(d goqu.DialectWrapper, res *systemType.Template) *goqu.InsertDataset {
		return d.Insert(templateTable).
			Rows(goqu.Record{
				"id":           res.ID,
				"rel_owner":    res.OwnerID,
				"handle":       res.Handle,
				"language":     res.Language,
				"type":         res.Type,
				"partial":      res.Partial,
				"meta":         res.Meta,
				"template":     res.Template,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"deleted_at":   res.DeletedAt,
				"last_used_at": res.LastUsedAt,
			})
	}

	// templateUpsertQuery assembles (insert+on-conflict) query for replacing templates
	//
	// This function is auto-generated
	templateUpsertQuery = func(d goqu.DialectWrapper, res *systemType.Template) *goqu.InsertDataset {
		var target = `,id`

		return templateInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"rel_owner":    res.OwnerID,
						"handle":       res.Handle,
						"language":     res.Language,
						"type":         res.Type,
						"partial":      res.Partial,
						"meta":         res.Meta,
						"template":     res.Template,
						"created_at":   res.CreatedAt,
						"updated_at":   res.UpdatedAt,
						"deleted_at":   res.DeletedAt,
						"last_used_at": res.LastUsedAt,
					},
				),
			)
	}

	// templateUpdateQuery assembles query for updating templates
	//
	// This function is auto-generated
	templateUpdateQuery = func(d goqu.DialectWrapper, res *systemType.Template) *goqu.UpdateDataset {
		return d.Update(templateTable).
			Set(goqu.Record{
				"rel_owner":    res.OwnerID,
				"handle":       res.Handle,
				"language":     res.Language,
				"type":         res.Type,
				"partial":      res.Partial,
				"meta":         res.Meta,
				"template":     res.Template,
				"created_at":   res.CreatedAt,
				"updated_at":   res.UpdatedAt,
				"deleted_at":   res.DeletedAt,
				"last_used_at": res.LastUsedAt,
			}).
			Where(templatePrimaryKeys(res))
	}

	// templateDeleteQuery assembles delete query for removing templates
	//
	// This function is auto-generated
	templateDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(templateTable).Where(ee...)
	}

	// templateDeleteQuery assembles delete query for removing templates
	//
	// This function is auto-generated
	templateTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(templateTable)
	}

	// templatePrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	templatePrimaryKeys = func(res *systemType.Template) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// userTable represents users store table
	//
	// This value is auto-generated
	userTable = goqu.T("users")

	// userSelectQuery assembles select query for fetching users
	//
	// This function is auto-generated
	userSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"email",
			"email_confirmed",
			"rel_user_group",
			"username",
			"name",
			"handle",
			"kind",
			"meta",
			"suspended_at",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(userTable)
	}

	// userInsertQuery assembles query inserting users
	//
	// This function is auto-generated
	userInsertQuery = func(d goqu.DialectWrapper, res *systemType.User) *goqu.InsertDataset {
		return d.Insert(userTable).
			Rows(goqu.Record{
				"id":              res.ID,
				"email":           res.Email,
				"email_confirmed": res.EmailConfirmed,
				"rel_user_group":  res.UserGroupID,
				"username":        res.Username,
				"name":            res.Name,
				"handle":          res.Handle,
				"kind":            res.Kind,
				"meta":            res.Meta,
				"suspended_at":    res.SuspendedAt,
				"created_at":      res.CreatedAt,
				"updated_at":      res.UpdatedAt,
				"deleted_at":      res.DeletedAt,
			})
	}

	// userUpsertQuery assembles (insert+on-conflict) query for replacing users
	//
	// This function is auto-generated
	userUpsertQuery = func(d goqu.DialectWrapper, res *systemType.User) *goqu.InsertDataset {
		var target = `,id`

		return userInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"email":           res.Email,
						"email_confirmed": res.EmailConfirmed,
						"rel_user_group":  res.UserGroupID,
						"username":        res.Username,
						"name":            res.Name,
						"handle":          res.Handle,
						"kind":            res.Kind,
						"meta":            res.Meta,
						"suspended_at":    res.SuspendedAt,
						"created_at":      res.CreatedAt,
						"updated_at":      res.UpdatedAt,
						"deleted_at":      res.DeletedAt,
					},
				),
			)
	}

	// userUpdateQuery assembles query for updating users
	//
	// This function is auto-generated
	userUpdateQuery = func(d goqu.DialectWrapper, res *systemType.User) *goqu.UpdateDataset {
		return d.Update(userTable).
			Set(goqu.Record{
				"email":           res.Email,
				"email_confirmed": res.EmailConfirmed,
				"rel_user_group":  res.UserGroupID,
				"username":        res.Username,
				"name":            res.Name,
				"handle":          res.Handle,
				"kind":            res.Kind,
				"meta":            res.Meta,
				"suspended_at":    res.SuspendedAt,
				"created_at":      res.CreatedAt,
				"updated_at":      res.UpdatedAt,
				"deleted_at":      res.DeletedAt,
			}).
			Where(userPrimaryKeys(res))
	}

	// userDeleteQuery assembles delete query for removing users
	//
	// This function is auto-generated
	userDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(userTable).Where(ee...)
	}

	// userDeleteQuery assembles delete query for removing users
	//
	// This function is auto-generated
	userTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(userTable)
	}

	// userPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	userPrimaryKeys = func(res *systemType.User) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}

	// userGroupTable represents userGroups store table
	//
	// This value is auto-generated
	userGroupTable = goqu.T("user_groups")

	// userGroupSelectQuery assembles select query for fetching userGroups
	//
	// This function is auto-generated
	userGroupSelectQuery = func(d goqu.DialectWrapper) *goqu.SelectDataset {
		return d.Select(
			"id",
			"handle",
			"meta",
			"config",
			"archived_at",
			"created_at",
			"updated_at",
			"deleted_at",
		).From(userGroupTable)
	}

	// userGroupInsertQuery assembles query inserting userGroups
	//
	// This function is auto-generated
	userGroupInsertQuery = func(d goqu.DialectWrapper, res *systemType.UserGroup) *goqu.InsertDataset {
		return d.Insert(userGroupTable).
			Rows(goqu.Record{
				"id":          res.ID,
				"handle":      res.Handle,
				"meta":        res.Meta,
				"config":      res.Config,
				"archived_at": res.ArchivedAt,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
				"deleted_at":  res.DeletedAt,
			})
	}

	// userGroupUpsertQuery assembles (insert+on-conflict) query for replacing userGroups
	//
	// This function is auto-generated
	userGroupUpsertQuery = func(d goqu.DialectWrapper, res *systemType.UserGroup) *goqu.InsertDataset {
		var target = `,id`

		return userGroupInsertQuery(d, res).
			OnConflict(
				goqu.DoUpdate(target[1:],
					goqu.Record{
						"handle":      res.Handle,
						"meta":        res.Meta,
						"config":      res.Config,
						"archived_at": res.ArchivedAt,
						"created_at":  res.CreatedAt,
						"updated_at":  res.UpdatedAt,
						"deleted_at":  res.DeletedAt,
					},
				),
			)
	}

	// userGroupUpdateQuery assembles query for updating userGroups
	//
	// This function is auto-generated
	userGroupUpdateQuery = func(d goqu.DialectWrapper, res *systemType.UserGroup) *goqu.UpdateDataset {
		return d.Update(userGroupTable).
			Set(goqu.Record{
				"handle":      res.Handle,
				"meta":        res.Meta,
				"config":      res.Config,
				"archived_at": res.ArchivedAt,
				"created_at":  res.CreatedAt,
				"updated_at":  res.UpdatedAt,
				"deleted_at":  res.DeletedAt,
			}).
			Where(userGroupPrimaryKeys(res))
	}

	// userGroupDeleteQuery assembles delete query for removing userGroups
	//
	// This function is auto-generated
	userGroupDeleteQuery = func(d goqu.DialectWrapper, ee ...goqu.Expression) *goqu.DeleteDataset {
		return d.Delete(userGroupTable).Where(ee...)
	}

	// userGroupDeleteQuery assembles delete query for removing userGroups
	//
	// This function is auto-generated
	userGroupTruncateQuery = func(d goqu.DialectWrapper) *goqu.TruncateDataset {
		return d.Truncate(userGroupTable)
	}

	// userGroupPrimaryKeys assembles set of conditions for all primary keys
	//
	// This function is auto-generated
	userGroupPrimaryKeys = func(res *systemType.UserGroup) goqu.Ex {
		return goqu.Ex{
			"id": res.ID,
		}
	}
)
