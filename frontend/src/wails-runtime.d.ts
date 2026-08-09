declare module '/wails/runtime.js' {
  export const Call: {
    ByName<T = unknown>(name: string, ...args: unknown[]): Promise<T>;
  };
}
