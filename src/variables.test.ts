import type { TemplateSrv } from '@grafana/runtime';
import { replaceVariables } from './variables';
import type { GitHubQuery } from './types/query';

// Model TemplateSrv's public formatter/metadata contract. Selections remain
// arrays until the formatter is invoked, including values containing commas.
function templateWithSelections(values: Record<string, string | string[] | undefined>) {
  const replace: TemplateSrv['replace'] = (input = '', scoped = {}, format, interpolations) =>
    input.replace(/\$(\w+)|\$\{(\w+)(?::(\w+))?\}/g, (match, bare, braced, explicitFormat) => {
      const name = bare || braced;
      const selection = scoped[name]?.value ?? values[name];
      if (selection === undefined) {
        interpolations?.push({ match, variableName: name, value: match, found: false });
        return match;
      }
      const selectedFormat = explicitFormat || format;
      const value =
        typeof selectedFormat === 'function'
          ? selectedFormat(selection)
          : Array.isArray(selection)
            ? selection.join(',')
            : String(selection);
      interpolations?.push({ match, variableName: name, format: selectedFormat, value, found: true });
      return value;
    });
  return {
    replace: jest.fn(replace),
    getVariables: jest.fn(),
    updateTimeRange: jest.fn(),
    containsTemplate: jest.fn(),
  };
}

describe('variables', () => {
  it('should not interpolate refId', () => {
    const t = {
      replace: jest.fn((a) => a),
      getVariables: jest.fn(),
      updateTimeRange: jest.fn(),
      containsTemplate: jest.fn(),
    };
    const query: GitHubQuery = {
      refId: 'A',
      foo: 'bar',
      foo1: 123.45,
      query: 'myQuery',
      foo2: 'bar2',
    } as any as GitHubQuery;
    expect(replaceVariables(t, query, {})).toStrictEqual({ ...query });
    expect(t.replace).toHaveBeenCalledTimes(3);
    expect(t.replace).toHaveBeenNthCalledWith(1, 'bar', {}, undefined);
    expect(t.replace).toHaveBeenNthCalledWith(2, 'myQuery', {}, 'csv');
    expect(t.replace).toHaveBeenNthCalledWith(3, 'bar2', {}, undefined);
  });

  it('should interpolate query with options', () => {
    const t = {
      replace: jest.fn((a) => a),
      getVariables: jest.fn(),
      updateTimeRange: jest.fn(),
      containsTemplate: jest.fn(),
    };
    const query: GitHubQuery = {
      refId: 'A',
      foo: 'bar',
      foo1: 123.45,
      query: 'myQuery',
      foo2: 'bar2',
      options: { foo: 'options_bar', foo1: 123.45, query: 'options_myQuery', foo2: 'options_bar2' },
    } as any as GitHubQuery;
    expect(replaceVariables(t, query, {})).toStrictEqual({ ...query });
    expect(t.replace).toHaveBeenCalledTimes(6);
    expect(t.replace).toHaveBeenNthCalledWith(1, 'bar', {}, undefined);
    expect(t.replace).toHaveBeenNthCalledWith(2, 'myQuery', {}, 'csv');
    expect(t.replace).toHaveBeenNthCalledWith(3, 'bar2', {}, undefined);
    expect(t.replace).toHaveBeenNthCalledWith(4, 'options_bar', {}, undefined);
    expect(t.replace).toHaveBeenNthCalledWith(5, 'options_myQuery', {}, 'csv');
    expect(t.replace).toHaveBeenNthCalledWith(6, 'options_bar2', {}, undefined);
  });

  it('should interpolate string arrays and expand multi-value variables', () => {
    const t = templateWithSelections({ branches: ['fix/b', 'fix/c'] });
    const query = {
      refId: 'A',
      queryType: 'Pull_Requests_For_Branches',
      options: { branches: ['feat/a', '$branches'] },
    } as GitHubQuery;
    const result: any = replaceVariables(t, query, {});
    expect(result.options.branches).toEqual(['feat/a', 'fix/b', 'fix/c']);
  });

  it('should keep literal commas in string array items', () => {
    const t = {
      replace: jest.fn((a: string) => a),
      getVariables: jest.fn(),
      updateTimeRange: jest.fn(),
      containsTemplate: jest.fn(),
    };
    const query = {
      refId: 'A',
      queryType: 'Pull_Requests_For_Branches',
      options: { branches: ['feat/a,b', ''] },
    } as GitHubQuery;
    const result: any = replaceVariables(t, query, {});
    expect(result.options.branches).toEqual(['feat/a,b', '']);
  });

  it.each([
    { input: '$branch', values: { branch: 'feat/a,b' }, expected: ['feat/a,b'] },
    { input: '${branches:csv}', values: { branches: ['feat/a,b', 'fix/c,d'] }, expected: ['feat/a,b', 'fix/c,d'] },
    { input: '${branches:raw}', values: { branches: ['feat/a,b', 'fix/c'] }, expected: ['feat/a,b', 'fix/c'] },
    {
      input: 'prefix/$branches/suffix',
      values: { branches: ['a,b', 'c'] },
      expected: ['prefix/a,b/suffix', 'prefix/c/suffix'],
    },
    { input: '$branches/$branches', values: { branches: ['a,b', 'c'] }, expected: ['a,b/a,b', 'c/c'] },
    { input: '${repositories:csv}', values: { repositories: ['repo-a', 'repo-b'] }, expected: ['repo-a', 'repo-b'] },
    { input: '$branches', values: { branches: [] }, expected: [] },
    { input: '$unknown', values: {}, expected: ['$unknown'] },
  ])('preserves actual selection boundaries for $input ($expected)', ({ input, values, expected }) => {
    const t = templateWithSelections(values);
    const query: GitHubQuery = {
      refId: 'A',
      queryType: 'Pull_Requests_For_Branches',
      options: { branches: [input] },
    };
    const result = replaceVariables(t, query, {});
    expect(result.options).toEqual({ branches: expected });
  });

  it('uses a scoped scalar selection without splitting its comma', () => {
    const t = templateWithSelections({ branches: ['other/a', 'other/b'] });
    const query: GitHubQuery = {
      refId: 'A',
      queryType: 'Pull_Requests_For_Branches',
      options: { branches: ['$branches'] },
    };
    const result = replaceVariables(t, query, { branches: { value: 'feat/a,b', text: 'feat/a,b' } });
    expect(result.options).toEqual({ branches: ['feat/a,b'] });
  });

  it('should interpolate variables in options as well', () => {
    const t = {
      replace: jest.fn((a: string) => a.slice(1, a.length)),
      getVariables: jest.fn(),
      updateTimeRange: jest.fn(),
      containsTemplate: jest.fn(),
    };
    const query: GitHubQuery = {
      refId: 'A',
      foo: '$bar',
      query: 'myQuery',
      options: { foo: '$options_bar' },
    } as any as GitHubQuery;
    const result: any = replaceVariables(t, query, {});
    expect(result['foo']).toBe('bar');
    expect(result.options?.['foo']).toBe('options_bar');
  });
});
