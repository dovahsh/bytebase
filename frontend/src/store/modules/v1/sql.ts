import { ClientError, Status } from "nice-grpc-common";
import { defineStore } from "pinia";
import { sqlServiceClient } from "@/grpcweb";
import { SQLResultSetV1 } from "@/types";
import { ExportFormat } from "@/types/proto/v1/common";
import { ExportRequest, QueryRequest } from "@/types/proto/v1/sql_service";
import { extractGrpcErrorMessage } from "@/utils/grpcweb";

export const useSQLStore = defineStore("sql", () => {
  const queryReadonly = async (
    params: QueryRequest
  ): Promise<SQLResultSetV1> => {
    try {
      const response = await sqlServiceClient.query(params, {
        // Skip global error handling since we will handle and display
        // errors manually.
        ignoredCodes: [Status.PERMISSION_DENIED],
        silent: true,
      });

      return {
        error: "",
        ...response,
      };
    } catch (err) {
      const error = extractGrpcErrorMessage(err);
      const status = err instanceof ClientError ? err.code : Status.UNKNOWN;
      return {
        error,
        results: [],
        advices: [],
        allowExport: false,
        status,
      };
    }
  };

  const exportData = async (params: ExportRequest) => {
    return await sqlServiceClient.export(params, {
      // Won't jump to 403 page when permission denied.
      ignoredCodes: [Status.PERMISSION_DENIED],
    });
  };

  return {
    queryReadonly,
    exportData,
  };
});

export const getExportRequestFormat = (
  format: "CSV" | "JSON" | "SQL" | "XLSX"
): ExportFormat => {
  switch (format) {
    case "CSV":
      return ExportFormat.CSV;
    case "JSON":
      return ExportFormat.JSON;
    case "SQL":
      return ExportFormat.SQL;
    case "XLSX":
      return ExportFormat.XLSX;
    default:
      return ExportFormat.FORMAT_UNSPECIFIED;
  }
};

export const getExportFileType = (format: "CSV" | "JSON" | "SQL" | "XLSX") => {
  switch (format) {
    case "CSV":
      return "text/csv";
    case "JSON":
      return "application/json";
    case "SQL":
      return "application/sql";
    case "XLSX":
      return "application/vnd.ms-excel";
  }
};
