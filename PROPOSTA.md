# PANDA Coin — A proposta: uma moeda digital nas mãos de muitos

## A visão

A PANDA Coin nasce de uma pergunta simples: e se qualquer pessoa na Terra
pudesse participar de uma rede de dinheiro digital usando apenas o computador
que já tem em casa? Não um investidor com capital para montar um galpão de
máquinas. Não um data center. Uma pessoa comum, com um notebook usado, um PC
antigo ou um Raspberry Pi de duzentos reais ligado atrás da televisão.

A proposta é construir uma criptomoeda que segue os paradigmas que fizeram o
Bitcoin funcionar — prova de trabalho, recompensa que cai pela metade ao longo
do tempo, nenhuma autoridade central — mas corrigindo o que o tempo revelou
ser a maior falha prática do Bitcoin: a mineração, que deveria ser de todos,
virou um clube fechado de quem pode pagar por hardware especializado e contas
de energia industriais.

O princípio norteador do projeto cabe numa frase: **um node em cada casa**.
Toda decisão técnica, do algoritmo de mineração ao tamanho dos blocos, é
julgada por uma única régua — isso deixa mais fácil ou mais difícil ter um
node rodando na casa de uma pessoa comum? Se dificulta, está errado, por mais
elegante que seja.

## O problema: como o dinheiro descentralizado centralizou

Para entender a proposta, é preciso entender o que deu errado no caminho do
Bitcoin — não no design, mas nas consequências dele.

O Bitcoin resolve um problema profundo: como milhares de computadores que não
se conhecem e não confiam uns nos outros podem concordar sobre quem tem
quanto dinheiro, sem um banco no meio? A resposta foi a prova de trabalho.
Para escrever a próxima página do livro-razão — o próximo bloco — um
computador precisa vencer uma loteria matemática: testar bilhões de números
até encontrar um que, combinado com os dados do bloco, produza um resultado
raro. Quem encontra primeiro, escreve o bloco e recebe moedas novas como
recompensa. Fraudar a história exigiria refazer todo esse trabalho, mais
rápido que o resto do mundo somado. É um design brilhante.

O problema está no algoritmo escolhido para essa loteria: o SHA-256, uma
função de cálculo puro, que usa quase nenhuma memória — só operações lógicas
simples, repetidas bilhões de vezes. E qualquer tarefa assim, simples e
repetitiva, pode ser gravada em silício. Surgiram os ASICs: chips que fazem
uma única coisa na vida — calcular SHA-256 — e fazem isso milhões de vezes
mais rápido e com muito menos energia que qualquer processador comum.

As consequências foram brutais. Um notebook calcula cerca de dez milhões de
tentativas por segundo; um ASIC de dois mil dólares calcula duzentos
trilhões. A chance de uma pessoa comum minerar um bloco em casa tornou-se
efetivamente zero. A mineração migrou para onde há capital e energia barata,
concentrou-se em poucas empresas e poucos países, e a rede que nasceu para
tirar o dinheiro das mãos de poucos entregou sua segurança a um punhado de
galpões industriais. Junto veio a corrida armamentista de energia: como
hardware melhor sempre vence, a competição vira quem queima mais eletricidade.

A PANDA Coin não abandona a prova de trabalho — ela continua sendo a forma
mais testada de garantir uma rede sem dono. A proposta é trocar a natureza do
trabalho.

## A solução: trabalho limitado por memória, não por cálculo

A ideia central chama-se memory-hard, ou dureza de memória. Em vez de um
algoritmo de cálculo puro, a loteria da PANDA Coin usa o Argon2id — um
algoritmo criado originalmente para proteger senhas, vencedor de uma
competição internacional de criptografia em 2015, projetado exatamente para
frustrar atacantes com hardware especializado.

Funciona assim: para calcular uma única tentativa da loteria, o computador é
obrigado a preencher sessenta e quatro megabytes de memória RAM com dados
pseudo-aleatórios e depois percorrê-los em uma ordem que depende dos próprios
dados — ou seja, não dá para saber qual pedaço da memória será necessário
antes de calcular o anterior. Não existe atalho: ou você usa a memória toda,
ou fica absurdamente mais lento, e essa troca é garantida matematicamente.

Isso muda o gargalo. No SHA-256, vence quem calcula mais rápido — e chips
dedicados calculam incomparavelmente mais rápido. No Argon2id, vence quem
acessa memória mais rápido — e a memória RAM é uma commodity: a do seu
notebook tem essencialmente a mesma velocidade da de um servidor de luxo. Um
suposto "ASIC de Argon2" precisaria embutir grandes quantidades de RAM
rápida, o que é, na prática, construir um computador comum, pelo preço de um
computador comum. A vantagem do hardware especializado despenca de um milhão
de vezes para duas a cinco vezes — e ninguém investe milhões para fabricar um
chip que rende três vezes mais que um notebook usado.

Essa não é uma aposta teórica. O Monero, uma das maiores criptomoedas em
operação, adotou essa filosofia em 2019 com um algoritmo chamado RandomX,
justamente para expulsar os ASICs da sua rede — e funcionou: é hoje a maior
rede do mundo minerada por gente comum em processadores comuns. A PANDA Coin
segue o mesmo caminho com o Argon2id, que entrega o mesmo efeito com uma
implementação muito mais simples e auditável.

E a energia? Aqui vale uma honestidade técnica importante. Nenhum design de
prova de trabalho limita o consumo total da rede — se um milhão de máquinas
minerarem, um milhão de processadores estarão trabalhando. O que o design
memory-hard garante é outra coisa, igualmente valiosa: o custo por
participante fica fixo e baixo — um núcleo de processador e sessenta e quatro
megabytes de RAM, o equivalente a deixar uma aba de navegador aberta — e,
principalmente, desaparece o incentivo econômico para escalar hardware.
Ninguém compra uma máquina mais faminta porque não existe máquina mais
faminta que ajude. A segurança da rede passa a crescer com a quantidade de
pessoas participando, não com a potência de cada uma. É a diferença entre uma
eleição e um leilão.

## Como a rede se mantém estável enquanto cresce

Uma rede assim precisa de um termostato. Na PANDA Coin, a meta é um bloco por
minuto na fase de desenvolvimento — dez minutos na rede definitiva, como no
Bitcoin. Mas se mais gente entra minerando, os blocos passam a sair rápido
demais; se gente sai, devagar demais.

O ajuste é automático e todos os nós fazem a mesma conta: a cada cem blocos,
a rede compara quanto tempo eles realmente levaram com quanto deveriam ter
levado. Se saíram rápido demais, a loteria fica proporcionalmente mais
difícil; se lentos demais, mais fácil — sempre com um limite de quatro vezes
por ajuste, para evitar solavancos e manipulação. O resultado é que o ritmo
de emissão de moedas é imune ao tamanho da rede: com cinco participantes ou
cinco milhões, o relógio da moeda bate no mesmo compasso.

## A escolha da mineração: pontos fortes, fracos e dificuldades honestas

Nenhuma escolha de engenharia é de graça, e um projeto que se pretende
honesto precisa falar dos dois lados da moeda que escolheu. A mineração
memory-hard tem forças reais — e fraquezas reais, com dificuldades que o
projeto vai ter que enfrentar de frente.

Comecemos pelas forças, para deixá-las claras de uma vez. A primeira é a
razão de existir do projeto: anular a vantagem do capital. Sem chip dedicado
que valha a pena fabricar, a mineração se distribui por quem aparece, não por
quem investe. A segunda é o custo de entrada: zero. Qualquer máquina dos
últimos dez anos participa, sem compra, sem instalação complexa, sem
eletricidade extra perceptível. A terceira é a natureza do algoritmo: o
Argon2id é aberto, padronizado, auditado por uma década de criptógrafos, e
sua implementação em Go é pequena o bastante para ser lida e entendida por
uma pessoa em uma tarde — segurança que se pode verificar, não que se
precisa acreditar.

Agora, as fraquezas — e aqui a conversa fica mais interessante.

A primeira dificuldade é o custo da verificação. No Bitcoin, conferir se um
bloco é válido custa um cálculo instantâneo; qualquer node checa milhões de
blocos em segundos. No Argon2id, verificar custa o mesmo que tentar: cerca de
um décimo de segundo e sessenta e quatro megabytes de memória por bloco. Para
uma rede com um bloco por minuto isso é tranquilo — mas cria um vetor de
ataque: alguém pode fabricar blocos falsos baratos de enviar e caros de
conferir, tentando afogar os nodes em verificações inúteis. O projeto mitiga
isso verificando primeiro tudo o que é barato — a estrutura, os
encadeamentos, as assinaturas — e deixando o teste caro por último, além de
limitar quanto trabalho não solicitado um vizinho pode nos impor. Mas é uma
tensão permanente do design, não um problema resolvido.

A segunda fraqueza é o lado sombrio da própria democratização: se qualquer
processador comum minera bem, então processadores roubados também mineram
bem. Moedas mineráveis em CPU historicamente atraem botnets — redes de
máquinas infectadas — e mineradores escondidos em sites e aplicativos. O
Monero convive com isso até hoje. Não existe solução técnica completa para
esse problema; existe vigilância da comunidade e a consciência de que é o
preço da acessibilidade. A alternativa — voltar ao hardware caro — mataria o
propósito do projeto para resolver um problema que ele pode apenas
administrar.

A terceira dificuldade é a infância da rede. A segurança da prova de
trabalho é proporcional ao trabalho total acumulado — e uma rede jovem, com
poucos participantes, tem pouco trabalho acumulado. Nos primeiros tempos, uma
única pessoa alugando algumas dezenas de servidores na nuvem poderia
concentrar mais da metade do poder da rede e, com isso, reescrever blocos
recentes. Todas as redes descentralizadas nasceram frágeis assim, inclusive o
Bitcoin, que nos primeiros anos era minerável por qualquer laptop e valia
nada — a fragilidade inicial é o pedágio de entrada. O projeto trata isso com
transparência: enquanto a rede for pequena, ela é um experimento entre
pessoas que confiam umas nas outras, não um cofre.

A quarta é a mais sutil: a resistência a hardware especializado não é um
teorema eterno, é uma corrida. As placas de vídeo modernas, por exemplo, têm
memória extremamente rápida, e dependendo dos parâmetros escolhidos podem
manter alguma vantagem sobre processadores comuns — menor que no SHA-256,
mas não zero. E a pesquisa em otimizações avança: o Monero precisou trocar de
algoritmo mais de uma vez quando fabricantes encontraram brechas. A PANDA
Coin herda essa realidade: os parâmetros do Argon2id — quanta memória,
quantas passadas — são decisões de consenso que podem precisar de revisão, e
uma troca dessas em rede viva é um evento delicado que exige a concordância
de todos os nodes. O compromisso do projeto não é "nunca vai centralizar", e
sim "vamos medir, e corrigir quando desviar".

Por fim, a honestidade energética já dita antes, que vale repetir no
contexto: a dificuldade ajusta a velocidade dos blocos, não o consumo total.
O que o design garante é que o consumo de cada casa é irrisório e que não há
prêmio para quem gastar mais. A conta total da rede cresce com o número de
participantes — como cresce a conta total de qualquer coisa que muita gente
faz junto.

Pesando tudo: as fraquezas são administráveis e conhecidas; a força — uma
rede aberta a qualquer pessoa — é única. É uma troca que o projeto faz de
olhos abertos.

## A economia: escassez programada

A política monetária da PANDA Coin é herdada do Bitcoin e é talvez a ideia
mais radical dele: uma moeda cuja emissão não pode ser alterada por nenhum
governo, empresa ou fundador, porque está escrita nas regras que todos os nós
verificam.

Cada bloco minerado cria moedas novas, pagas a quem o minerou — é a única
forma de PANDA nascer. Essa recompensa começa em cinquenta moedas por bloco e
cai pela metade em intervalos fixos: é o halving. Cinquenta viram vinte e
cinco, depois doze e meio, e assim por diante, até que a emissão
matematicamente se esgota. A soma de todos os blocos que existirão converge
para um teto fixo — na configuração de desenvolvimento, cerca de cem mil
moedas, um número que ninguém pode aumentar. Depois disso, quem minera é
remunerado pelas taxas das transações que inclui nos blocos.

Escassez digital verificável: qualquer pessoa com um node pode conferir, a
qualquer momento, exatamente quantas moedas existem e provar que nenhuma foi
criada fora das regras.

## Por dentro da máquina: como tudo se encaixa

Vale entender as peças, porque cada uma tem um papel na descentralização.

Uma blockchain é, literalmente, uma corrente de blocos: cada bloco carrega a
impressão digital criptográfica do anterior. Alterar qualquer bloco antigo
muda sua impressão digital, o que quebra o elo seguinte, e o seguinte, até a
ponta — reescrever o passado exige refazer toda a prova de trabalho dali em
diante, competindo contra a rede inteira. É isso que torna o histórico
imutável na prática.

O dinheiro em si segue o modelo do Bitcoin, chamado UTXO. Não existem contas
com saldo; existem moedas avulsas, como notas numa carteira, cada uma
trancada para o dono por criptografia. Pagar é entregar notas inteiras e
receber troco. Sua chave privada — um segredo que só você tem — é o que
destranca suas notas; a assinatura digital que ela produz prova a posse sem
revelar o segredo. Daí vem uma regra dura e libertadora ao mesmo tempo:
perder a chave é perder os fundos, mas nenhuma autoridade pode congelar ou
confiscar o que é seu.

As transações recém-enviadas esperam numa sala chamada mempool, e os
mineradores escolhem quais incluir no próximo bloco — tipicamente as que
pagam mais taxa, um leilão honesto pelo espaço. A comunicação entre os nós
funciona por fofoca: cada node conta as novidades aos poucos vizinhos que
conhece, que repassam aos seus, e em segundos a rede inteira sabe — sem
servidor central, sem ponto único de falha. Quando dois mineradores acham
blocos quase ao mesmo tempo e a rede momentaneamente se divide, os nós
resolvem sozinhos: vale a versão da história com mais trabalho acumulado, e
todos convergem em poucos blocos, sem ninguém no comando.

## Um node em cada casa, levado a sério

Esse princípio vira números concretos no projeto. O software é um único
arquivo executável, sem instalação, sem dependências, compilado para Linux,
Mac e Windows, em processadores comuns e também em ARM — o do Raspberry Pi.
Rodando sem minerar, o node consome menos de cento e vinte e oito megabytes
de memória; minerando com a configuração padrão, cerca de sessenta e quatro a
mais. O padrão é minerar com um único núcleo do processador — participar da
rede não deve atrapalhar o uso normal da máquina; quem quiser doar mais
núcleos, aumenta na configuração.

O tamanho dos blocos é limitado pelas próprias regras de consenso — duzentos
e cinquenta e seis kilobytes — precisamente para que o disco necessário para
guardar a cadeia inteira cresça devagar, na casa de poucos gigabytes por ano
no pior cenário. Parece um detalhe, mas é uma decisão de soberania: se
guardar a história da rede exigisse um disco de servidor, só empresas
rodariam nodes completos, e a verificação — o ato de não precisar confiar em
ninguém — viraria privilégio.

E há o detalhe do roteador doméstico: a maioria das casas está atrás de NAT,
sem endereço acessível de fora. Na PANDA Coin, um node que apenas abre
conexões de saída é cidadão pleno — valida tudo, minera, propaga transações e
blocos — sem configurar absolutamente nada no roteador. Abrir portas para
receber conexões é opcional, uma doação extra à saúde da rede.

## O que o projeto é, e o que ainda não é

A primeira versão entrega a rede completa: os blocos e as transações, a prova
de trabalho memory-hard, o ajuste de dificuldade, o halving, a carteira com
chaves e endereços, a fila de transações, a comunicação entre nós, a
sincronização de um node novo do zero até a ponta da cadeia, e o minerador de
baixo consumo. Tudo em Go, tudo auditável, construído em cinco etapas que se
testam individualmente — dos tipos fundamentais até a demonstração final de
dois nodes na mesma máquina, um minerando e o outro sincronizando e recebendo
uma transferência.

Fora do escopo inicial, com honestidade: não há contratos inteligentes nem
sistema de scripts — a moeda faz uma coisa, transferir valor, e faz simples.
Não há criptografia do arquivo da carteira na primeira versão, nem travessia
automática de NAT — que já está no roadmap como prioridade seguinte, junto
com proteções contra nós maliciosos. A rede começa como uma rede de
desenvolvimento, com ciclos encurtados — blocos a cada minuto, halving a cada
mil blocos — para que se possa observar em horas o que no Bitcoin leva anos,
antes de calibrar os parâmetros definitivos.

## Filosofia: um projeto cypherpunk

Existe uma tradição por trás deste projeto, mais antiga que o Bitcoin. Nos
anos noventa, um grupo de programadores e criptógrafos que se chamavam de
cypherpunks defendia uma ideia simples e radical: privacidade e liberdade na
era digital não virão de leis nem de promessas de empresas — virão de
matemática. Criptografia não pede permissão. E o lema deles era um chamado à
ação: cypherpunks escrevem código. Não fazem manifesto e esperam; constroem a
ferramenta e soltam no mundo.

A PANDA Coin se assume herdeira dessa tradição, e isso muda o que ela é. Não
nasce como produto, não tem empresa por trás, não promete retorno a ninguém.
Nasce como código aberto que qualquer pessoa pode ler, rodar, copiar e
modificar. A identidade na rede é um par de chaves que você mesmo gera no seu
computador — ninguém aprova seu cadastro, ninguém pode banir sua conta,
porque não há cadastro nem conta: há matemática que responde a quem tem a
chave. As regras da moeda não estão nos termos de uso de alguém; estão no
código que cada node verifica por conta própria. No espírito cypherpunk,
confiar é um último recurso — verificar é o padrão.

E há uma escolha filosófica deliberada que talvez seja a mais contracultural
de todas: esta moeda não nasce para ser dinheiro. Não há venda inicial, não
há moedas reservadas para fundadores, não há promessa de valorização — cada
PANDA que existir terá nascido de um bloco minerado por alguém, em algum
lugar, numa máquina comum. Se um dia ela valer alguma coisa, que seja porque
pessoas encontraram uso nela — não porque alguém convenceu outras de que
enriqueceriam. O experimento é anterior à pergunta do preço: é sobre
descobrir o que uma comunidade faz com valor digital escasso quando ele não
carrega, ainda, o peso do dinheiro.

## Sonhos futuros: um like que vale alguma coisa

E o que uma comunidade faria com isso? Aqui entra o sonho — declaradamente um
sonho, ainda sem forma final, e está tudo bem que seja assim.

Existe um protocolo chamado Nostr que é, talvez, o parente mais próximo da
PANDA Coin em espírito. É uma rede social sem dono: não há empresa, não há
servidor central, não há algoritmo decidindo o que você vê. Sua identidade é
um par de chaves criptográficas — o mesmo conceito da carteira da PANDA — e
suas mensagens são notas assinadas que viajam por retransmissores
independentes espalhados pelo mundo. Ninguém pode apagar sua conta porque sua
conta é sua chave. É a mesma filosofia, aplicada à palavra em vez do valor.

No Nostr de hoje já existe um gesto bonito: os zaps, pequenos agradecimentos
em bitcoin enviados a quem publicou algo que valeu a pena. O sonho da PANDA
Coin é habitar esse mesmo gesto de um jeito próprio: imagine um like que
custa alguma coisa — não muito, um valor simbólico, minerado no notebook de
quem o envia. Um like de PANDA não seria um clique infinito e gratuito como
nas redes de empresa, onde a aprovação é abundante e por isso não significa
nada. Seria um gesto escasso: alguém dedicou um pedaço da sua máquina, do seu
tempo, da sua participação na rede, e escolheu dar isso a você porque o que
você escreveu tocou alguém de verdade. Apreciação com peso, mas sem passar
pelo dinheiro tradicional — valor que nasce na comunidade e circula dentro
dela.

Talvez seja isso. Talvez seja outra coisa: reputação verificável, recompensa
para quem hospeda infraestrutura da própria rede, um selo de apoio a
projetos, um jogo. A honestidade do projeto neste ponto é total: ainda não se
sabe para que a PANDA vai servir — e isso não é uma fraqueza do plano, é o
plano. As melhores tecnologias descentralizadas não nasceram com um caso de
uso fechado; nasceram como ferramentas abertas nas mãos de gente curiosa, e o
uso emergiu. Primeiro se constrói a rede — leve, justa, aberta, nas mãos de
muitos. Depois se descobre, junto, o que ela quer ser. Cypherpunks escrevem
código; o mundo decide o resto.

## Por que isso importa

O Bitcoin provou que dinheiro sem banco é possível. Mas deixou uma lição
incômoda: descentralização não é um estado que se declara, é um equilíbrio
que se projeta — e qualquer vantagem de escala, por menor que seja, compõe ao
longo do tempo até virar concentração.

A PANDA Coin é um experimento sobre projetar esse equilíbrio desde a primeira
linha de código: escolher o algoritmo que anula a vantagem do capital, limitar
o apetite do software ao que uma casa comum oferece, tratar o notebook usado e
o Raspberry Pi como os cidadãos de primeira classe da rede. Se a tese estiver
certa, o resultado é uma rede cuja força não se mede em megawatts, mas em
lares — mais parecida com uma assembleia do que com uma indústria.

Uma moeda que qualquer pessoa pode verificar, minerar e guardar. Nas mãos de
muitos, porque foi desenhada, decisão por decisão, para caber nelas.
