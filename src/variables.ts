import { map, Observable } from 'rxjs';
import { TemplateSrv, type VariableInterpolation } from '@grafana/runtime';
import { ScopedVars, CustomVariableSupport, DataQueryResponse, DataQueryRequest, DataFrameView } from '@grafana/data';
import { GitHubDataSource } from './DataSource';
import VariableQueryEditor from 'views/VariableQueryEditor';
import type { GitHubQuery, GitHubVariableQuery } from './types/query';

export const replaceVariable = (
  t: TemplateSrv,
  value?: string,
  scoped?: ScopedVars,
  format?: string
): string | undefined => {
  return !!value ? t.replace(value, scoped, format) : value;
};

export const replaceVariables = (t: TemplateSrv, query: GitHubQuery, scoped: ScopedVars): GitHubQuery => {
  let updatedQuery = { ...query };
  updatedQuery = interpolateObject(updatedQuery, t, scoped);
  if (query.options) {
    updatedQuery = { ...updatedQuery, options: interpolateObject(query.options, t, scoped) };
  }
  return updatedQuery;
};

// Resolve actual array selections before applying their requested formatting.
// CSV delimiters cannot distinguish branch-name commas from selection boundaries.
const interpolateArrayItem = (t: TemplateSrv, input: string, scoped: ScopedVars): string[] => {
  const interpolations: VariableInterpolation[] = [];
  const formatted = t.replace(input, scoped, 'csv', interpolations);
  let scopes = [scoped];
  let expanded = false;
  for (const name of new Set(interpolations.map((item) => item.variableName))) {
    // Explicit transformations such as :regex, :json and field access retain
    // their scalar formatting rather than being reinterpreted as lists.
    if (
      interpolations.some(
        (item) =>
          item.variableName === name &&
          (item.fieldPath || (item.format && item.format !== 'csv' && item.format !== 'raw'))
      )
    ) {
      continue;
    }
    let selection: string | string[] | undefined;
    t.replace(`\${${name}}`, scoped, (value: string | string[]) => {
      selection = value;
      return '';
    });
    if (Array.isArray(selection)) {
      const values = selection;
      expanded = true;
      scopes = scopes.flatMap((vars) => values.map((value) => ({ ...vars, [name]: { value, text: value } })));
    }
  }
  return expanded ? scopes.map((vars) => t.replace(input, vars, 'csv')) : [formatted];
};

const interpolateObject = (input: any, t: TemplateSrv, scoped: ScopedVars = {}) => {
  let newOptions = { ...input };
  Object.keys(newOptions).forEach((key) => {
    if (key !== 'refId') {
      const option = newOptions[key];
      if (typeof option === 'string') {
        if (key === 'query') {
          newOptions = { ...newOptions, [key]: replaceVariable(t, option, scoped, 'csv') };
        } else {
          newOptions = { ...newOptions, [key]: replaceVariable(t, option, scoped) };
        }
      } else if (Array.isArray(option)) {
        const replaced = option.flatMap((opt) => {
          if (typeof opt !== 'string') {
            return [interpolateObject(opt, t, scoped)];
          }
          return interpolateArrayItem(t, opt, scoped);
        });
        newOptions = { ...newOptions, [key]: replaced };
      }
    }
  });
  return newOptions;
};

export class GithubVariableSupport extends CustomVariableSupport<GitHubDataSource, GitHubVariableQuery, GitHubQuery> {
  constructor(private readonly datasource: GitHubDataSource) {
    super();
    this.datasource = datasource;
    this.query = this.query.bind(this);
  }
  editor = VariableQueryEditor;
  query(request: DataQueryRequest<GitHubVariableQuery>): Observable<DataQueryResponse> {
    let query = { ...request?.targets[0], refId: 'metricFindQuery' };
    return this.datasource
      .query({ ...request, targets: [query] })
      .pipe(map((response) => ({ ...response, data: response.data || [] })))
      .pipe(map((response) => queryResponseToVariablesFrame(query, response)));
  }
}

const queryResponseToVariablesFrame = (query: GitHubVariableQuery, response: DataQueryResponse) => {
  if (response?.data?.length < 1) {
    return { ...response, data: [] };
  }
  const view = new DataFrameView(response.data[0] || {});
  const data = view.map((item) => {
    const value = item[query.key || ''] || item[query.field || 'name'];
    const text = item[query.field || 'name'] || value;
    return { value, text };
  });
  return { ...response, data };
};
