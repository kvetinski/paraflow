use std::{env, io, process};

fn main() {
    let args = env::args_os().skip(1).collect::<Vec<_>>();
    let stdin = io::stdin();
    let mut input = stdin.lock();
    let exit_code =
        paraflow_engine::run_with_input(&args, &mut input, &mut io::stdout(), &mut io::stderr());
    process::exit(exit_code);
}
